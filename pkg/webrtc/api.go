package webrtc

import (
	"fmt"
	"net"
	"slices"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xnet"
	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

// ReceiveMTU = Ethernet MTU (1500) - IP Header (20) - UDP Header (8)
// https://ffmpeg.org/ffmpeg-all.html#Muxer
const ReceiveMTU = 1472

func NewAPI() (*webrtc.API, error) {
	return NewServerAPI("", "", nil)
}

type Filters struct {
	Candidates []string `yaml:"candidates"`
	Loopback   bool     `yaml:"loopback"`
	Interfaces []string `yaml:"interfaces"`
	IPs        []string `yaml:"ips"`
	Networks   []string `yaml:"networks"`
	UDPPorts   []uint16 `yaml:"udp_ports"`
}

func (f *Filters) Network(protocol string) string {
	if f == nil || f.Networks == nil {
		return protocol
	}
	v4 := slices.Contains(f.Networks, protocol+"4")
	v6 := slices.Contains(f.Networks, protocol+"6")
	if v4 && v6 {
		return protocol
	} else if v4 {
		return protocol + "4"
	} else if v6 {
		return protocol + "6"
	}
	return ""
}

func (f *Filters) NetIPs() (ips []net.IP) {
	itfs, _ := net.Interfaces()
	for _, itf := range itfs {
		if itf.Flags&net.FlagUp == 0 {
			continue
		}
		if !f.IncludeLoopback() && itf.Flags&net.FlagLoopback != 0 {
			continue
		}
		if !f.InterfaceFilter(itf.Name) {
			continue
		}

		addrs, _ := itf.Addrs()
		for _, addr := range addrs {
			ip := parseNetAddr(addr)
			if ip == nil || !f.IPFilter(ip) {
				continue
			}
			ips = append(ips, ip)
		}
	}
	return
}

func parseNetAddr(addr net.Addr) net.IP {
	switch addr := addr.(type) {
	case *net.IPNet:
		return addr.IP
	case *net.IPAddr:
		return addr.IP
	}
	return nil
}

func (f *Filters) IncludeLoopback() bool {
	return f != nil && f.Loopback
}

func (f *Filters) InterfaceFilter(name string) bool {
	return f == nil || f.Interfaces == nil || slices.Contains(f.Interfaces, name)
}

func (f *Filters) IPFilter(ip net.IP) bool {
	return f == nil || f.IPs == nil || core.Contains(f.IPs, ip.String())
}

func NewServerAPI(network, address string, filters *Filters) (*webrtc.API, error) {
	// for debug logs add to env: `PION_LOG_DEBUG=all`
	m := &webrtc.MediaEngine{}
	//if err := m.RegisterDefaultCodecs(); err != nil {
	//	return nil, err
	//}
	if err := RegisterDefaultCodecs(m); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, err
	}

	s := webrtc.SettingEngine{}

	// fix https://github.com/pion/webrtc/pull/2407
	s.SetDTLSInsecureSkipHelloVerify(true)

	if filters != nil && filters.Loopback {
		s.SetIncludeLoopbackCandidate(true)
	}

	var interfaceFilter func(name string) bool
	if filters != nil && filters.Interfaces != nil {
		interfaceFilter = func(name string) bool {
			return core.Contains(filters.Interfaces, name)
		}
	} else {
		// default interfaces - all, except loopback
	}
	s.SetInterfaceFilter(interfaceFilter)

	var ipFilter func(ip net.IP) bool
	if filters != nil && filters.IPs != nil {
		ipFilter = func(ip net.IP) bool {
			return core.Contains(filters.IPs, ip.String())
		}
	} else {
		// try filter all Docker-like interfaces
		ipFilter = func(ip net.IP) bool {
			return !xnet.Docker.Contains(ip)
		}
		// if there are no such interfaces - disable the filter
		// the user will need to enable port forwarding
		if nets, _ := xnet.IPNets(ipFilter); len(nets) == 0 {
			ipFilter = nil
		}
	}
	s.SetIPFilter(ipFilter)

	var networkTypes []webrtc.NetworkType
	if filters != nil && filters.Networks != nil {
		for _, s := range filters.Networks {
			if networkType, err := webrtc.NewNetworkType(s); err == nil {
				networkTypes = append(networkTypes, networkType)
			}
		}
	} else {
		// default network types - all
		networkTypes = []webrtc.NetworkType{
			webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6,
			webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6,
		}
	}
	s.SetNetworkTypes(networkTypes)

	if filters != nil && len(filters.UDPPorts) == 2 {
		_ = s.SetEphemeralUDPPortRange(filters.UDPPorts[0], filters.UDPPorts[1])
	}

	// If you don't specify an address, this won't cause an error.
	// Connections can still be established using random UDP addresses.
	if address != "" {
		// Both newMux functions respect filters and do not raise an error
		// if the port cannot be listened on.
		if network == "" || network == "tcp" {
			tcpMux := newTCPMux(address, filters)
			s.SetICETCPMux(tcpMux)
		}
		if network == "" || network == "udp" {
			udpMux := newUDPMux(address, filters)
			s.SetICEUDPMux(udpMux)
		}
	}

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(s),
	), nil
}

// OnNewListener temporary ugly solution for log
var OnNewListener = func(ln any) {}

func newTCPMux(address string, filters *Filters) ice.TCPMux {
	networkTCP := filters.Network("tcp") // tcp or tcp4 or tcp6
	if ln, _ := net.Listen(networkTCP, address); ln != nil {
		OnNewListener(ln)
		return webrtc.NewICETCPMux(nil, ln, 8)
	}
	return nil
}

func newUDPMux(address string, filters *Filters) ice.UDPMux {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil
	}

	// UDPMux should not listening on unspecified address.
	// So we will create a listener on all available interfaces.
	// We can't use ice.NewMultiUDPMuxFromPort, because it sometimes crashes with an error:
	//     listen udp [***]:8555: bind: cannot assign requested address
	var addrs []string
	if host == "" {
		for _, ip := range filters.NetIPs() {
			addrs = append(addrs, fmt.Sprintf("%s:%s", ip, port))
		}
	} else {
		addrs = []string{address}
	}

	networkUDP := filters.Network("udp") // udp or udp4 or udp6

	var muxes []ice.UDPMux
	for _, addr := range addrs {
		if ln, _ := net.ListenPacket(networkUDP, addr); ln != nil {
			OnNewListener(ln)
			mux := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: ln})
			muxes = append(muxes, mux)
		}
	}

	switch len(muxes) {
	case 0:
		return nil
	case 1:
		return muxes[0]
	}
	return ice.NewMultiUDPMuxDefault(muxes...)
}

func RegisterDefaultCodecs(m *webrtc.MediaEngine) error {
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
			},
			PayloadType: 101, //111,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType: webrtc.MimeTypePCMU, ClockRate: 8000,
			},
			PayloadType: 0,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType: webrtc.MimeTypePCMA, ClockRate: 8000,
			},
			PayloadType: 8,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{
		{"goog-remb", ""},
		{"ccm", "fir"},
		{"nack", ""},
		{"nack", "pli"},
	}
	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 96, // Chrome v110 - PayloadType: 102
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 97, // Chrome v110 - PayloadType: 106
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 98, // Chrome v110 - PayloadType: 112
		},
		// macOS Safari 15.1
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeH265,
				ClockRate:    90000,
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 100,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	return nil
}
