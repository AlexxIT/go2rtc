package webrtc

import (
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
	"net"
)

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

	s := webrtc.SettingEngine{
		//LoggerFactory: customLoggerFactory{},
	}

	// disable listen on Hassio docker interfaces
	s.SetInterfaceFilter(func(name string) bool {
		return name != "hassio" && name != "docker0"
	})

	// disable mDNS listener
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	if address != "" {
		s.SetNetworkTypes([]webrtc.NetworkType{
			webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6,
			webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6,
		})

		if ln, err := net.ListenPacket("udp", address); err == nil {
			udpMux := webrtc.NewICEUDPMux(nil, ln)
			s.SetICEUDPMux(udpMux)
		}

		if ln, err := net.Listen("tcp", address); err == nil {
			tcpMux := webrtc.NewICETCPMux(nil, ln, 8)
			s.SetICETCPMux(tcpMux)
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
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
			PayloadType:        101, //111,
		}, {
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypePCMU, 8000, 0, "", nil},
			PayloadType:        0,
		}, {
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypePCMA, 8000, 0, "", nil},
			PayloadType:        8,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}}
	for _, codec := range []webrtc.RTPCodecParameters{
		// macOS Google Chrome 103.0.5060.134
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f", videoRTCPFeedback},
			PayloadType:        96, //102,
		}, {
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f", videoRTCPFeedback},
			PayloadType:        97, //125,
		}, {
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032", videoRTCPFeedback},
			PayloadType:        98, //123,
		},
		// macOS Safari 15.1
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH265, 90000, 0, "", videoRTCPFeedback},
			PayloadType:        100,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	return nil
}
