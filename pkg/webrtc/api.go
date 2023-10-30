package webrtc

import (
	"net"
	"strings"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

// ReceiveMTU = Ethernet MTU (1500) - IP Header (20) - UDP Header (8)
// https://ffmpeg.org/ffmpeg-all.html#Muxer
const ReceiveMTU = 1472

func NewAPI(address string) (*webrtc.API, error) {
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

	// disable listen on Hassio docker interfaces
	s.SetInterfaceFilter(func(name string) bool {
		return name != "hassio" && name != "docker0"
	})

	// disable mDNS listener
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	// UDP6 may have problems with DNS resolving for STUN servers
	s.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeUDP6, webrtc.NetworkTypeTCP6,
	})

	// fix https://github.com/pion/webrtc/pull/2407
	s.SetDTLSInsecureSkipHelloVerify(true)

	s.SetReceiveMTU(ReceiveMTU)

	if address != "" {
		address, network, _ := strings.Cut(address, "/")
		if network == "" || network == "udp" {
			if ln, err := net.ListenPacket("udp", address); err == nil {
				udpMux := webrtc.NewICEUDPMux(nil, ln)
				s.SetICEUDPMux(udpMux)
			}
		}
		if network == "" || network == "tcp" {
			if ln, err := net.Listen("tcp", address); err == nil {
				tcpMux := webrtc.NewICETCPMux(nil, ln, 8)
				s.SetICETCPMux(tcpMux)
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
