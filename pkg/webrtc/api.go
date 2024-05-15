package webrtc

import (
	"net"
	"slices"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

// ReceiveMTU = Ethernet MTU (1500) - IP Header (20) - UDP Header (8)
// https://ffmpeg.org/ffmpeg-all.html#Muxer
const ReceiveMTU = 1472

func NewAPI() (*webrtc.API, error) {
	return NewServerAPI("", "", nil)
}

type Filters struct {
	Candidates []string `yaml:"candidates"`
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

	if filters != nil && filters.Interfaces != nil {
		s.SetIncludeLoopbackCandidate(true)
		s.SetInterfaceFilter(func(name string) bool {
			return slices.Contains(filters.Interfaces, name)
		})
	} else {
		// disable listen on Hassio docker interfaces
		s.SetInterfaceFilter(func(name string) bool {
			return name != "hassio" && name != "docker0"
		})
	}

	if filters != nil && filters.IPs != nil {
		s.SetIncludeLoopbackCandidate(true)
		s.SetIPFilter(func(ip net.IP) bool {
			return slices.Contains(filters.IPs, ip.String())
		})
	}

	if filters != nil && filters.Networks != nil {
		var networkTypes []webrtc.NetworkType
		for _, s := range filters.Networks {
			if networkType, err := webrtc.NewNetworkType(s); err == nil {
				networkTypes = append(networkTypes, networkType)
			}
		}
		s.SetNetworkTypes(networkTypes)
	} else {
		s.SetNetworkTypes([]webrtc.NetworkType{
			webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6,
			webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6,
		})
	}

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
			if ln, err := net.ListenPacket("udp", address); err == nil {
				udpMux := webrtc.NewICEUDPMux(nil, ln)
				s.SetICEUDPMux(udpMux)
			}
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
