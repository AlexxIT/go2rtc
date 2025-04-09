package webrtc

import (
	"net"

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

	//if len(hosts) != 0 {
	//	// support only: host, srflx
	//	if candidateType, err := webrtc.NewICECandidateType(hosts[0]); err == nil {
	//		s.SetNAT1To1IPs(hosts[1:], candidateType)
	//	} else {
	//		s.SetNAT1To1IPs(hosts, 0) // 0 = host
	//	}
	//}

	if address != "" {
		if network == "" || network == "tcp" {
			if ln, err := net.Listen("tcp", address); err == nil {
				tcpMux := webrtc.NewICETCPMux(nil, ln, 8)
				s.SetICETCPMux(tcpMux)
			}
		}

		if network == "" || network == "udp" {
			// UDPMuxDefault should not listening on unspecified address, use NewMultiUDPMuxFromPort instead
			var udpMux ice.UDPMux
			if port := xnet.ParseUnspecifiedPort(address); port != 0 {
				var networks []ice.NetworkType
				for _, ntype := range networkTypes {
					networks = append(networks, ice.NetworkType(ntype))
				}

				udpMux, _ = ice.NewMultiUDPMuxFromPort(
					port,
					ice.UDPMuxFromPortWithInterfaceFilter(interfaceFilter),
					ice.UDPMuxFromPortWithIPFilter(ipFilter),
					ice.UDPMuxFromPortWithNetworks(networks...),
				)
			} else if ln, err := net.ListenPacket("udp", address); err == nil {
				udpMux = ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: ln})
			}
			s.SetICEUDPMux(udpMux)
		}
	}

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(s),
	), nil
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
