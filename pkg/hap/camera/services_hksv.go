package camera

import (
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

func ServiceMotionSensor() *hap.Service {
	return &hap.Service{
		Type: "85",
		Characters: []*hap.Character{
			{
				Type:   "22",
				Format: hap.FormatBool,
				Value:  false,
				Perms:  hap.EVPR,
			},
			{
				Type:   "75",
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPR,
			},
		},
	}
}

func ServiceCameraOperatingMode() *hap.Service {
	return &hap.Service{
		Type: "21A",
		Characters: []*hap.Character{
			{
				Type:   "21B",
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   "223",
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   "225",
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPRPW,
			},
		},
	}
}

func ServiceCameraEventRecordingManagement() *hap.Service {
	val205, _ := tlv8.MarshalBase64(SupportedCameraRecordingConfiguration{
		PrebufferLength:     4000,
		EventTriggerOptions: 0x01, // motion
		MediaContainerConfigurations: MediaContainerConfigurations{
			MediaContainerType: 0, // fragmented MP4
			MediaContainerParameters: MediaContainerParameters{
				FragmentLength: 4000,
			},
		},
	})

	val206, _ := tlv8.MarshalBase64(SupportedVideoRecordingConfiguration{
		CodecConfigs: []VideoRecordingCodecConfiguration{
			{
				CodecType: VideoCodecTypeH264,
				CodecParams: VideoRecordingCodecParameters{
					ProfileID:      VideoCodecProfileHigh,
					Level:          VideoCodecLevel40,
					Bitrate:        2000,
					IFrameInterval: 4000,
				},
				CodecAttrs: VideoCodecAttributes{Width: 1920, Height: 1080, Framerate: 30},
			},
			{
				CodecType: VideoCodecTypeH264,
				CodecParams: VideoRecordingCodecParameters{
					ProfileID:      VideoCodecProfileMain,
					Level:          VideoCodecLevel31,
					Bitrate:        1000,
					IFrameInterval: 4000,
				},
				CodecAttrs: VideoCodecAttributes{Width: 1280, Height: 720, Framerate: 30},
			},
		},
	})

	val207, _ := tlv8.MarshalBase64(SupportedAudioRecordingConfiguration{
		CodecConfigs: []AudioRecordingCodecConfiguration{
			{
				CodecType: AudioRecordingCodecTypeAACLC,
				CodecParams: []AudioRecordingCodecParameters{
					{
						Channels:        1,
						BitrateMode:     []byte{AudioCodecBitrateVariable},
						SampleRate:      []byte{AudioRecordingSampleRate24Khz, AudioRecordingSampleRate32Khz, AudioRecordingSampleRate48Khz},
						MaxAudioBitrate: []uint32{64},
					},
				},
			},
		},
	})

	// Default selected recording configuration (Home Hub expects this to persist)
	val209, _ := tlv8.MarshalBase64(SelectedCameraRecordingConfiguration{
		GeneralConfig: SupportedCameraRecordingConfiguration{
			PrebufferLength:     4000,
			EventTriggerOptions: 0x01, // motion
			MediaContainerConfigurations: MediaContainerConfigurations{
				MediaContainerType: 0,
				MediaContainerParameters: MediaContainerParameters{
					FragmentLength: 4000,
				},
			},
		},
		VideoConfig: SupportedVideoRecordingConfiguration{
			CodecConfigs: []VideoRecordingCodecConfiguration{
				{
					CodecType: VideoCodecTypeH264,
					CodecParams: VideoRecordingCodecParameters{
						ProfileID:      VideoCodecProfileHigh,
						Level:          VideoCodecLevel40,
						Bitrate:        2000,
						IFrameInterval: 4000,
					},
					CodecAttrs: VideoCodecAttributes{Width: 1920, Height: 1080, Framerate: 30},
				},
			},
		},
		AudioConfig: SupportedAudioRecordingConfiguration{
			CodecConfigs: []AudioRecordingCodecConfiguration{
				{
					CodecType: AudioRecordingCodecTypeAACLC,
					CodecParams: []AudioRecordingCodecParameters{
						{
							Channels:        1,
							BitrateMode:     []byte{AudioCodecBitrateVariable},
							SampleRate:      []byte{AudioRecordingSampleRate24Khz},
							MaxAudioBitrate: []uint32{64},
						},
					},
				},
			},
		},
	})

	return &hap.Service{
		Type: "204",
		Characters: []*hap.Character{
			{
				Type:   "B0",
				Format: hap.FormatUInt8,
				Value:  0,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   TypeSupportedCameraRecordingConfiguration,
				Format: hap.FormatTLV8,
				Value:  val205,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeSupportedVideoRecordingConfiguration,
				Format: hap.FormatTLV8,
				Value:  val206,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeSupportedAudioRecordingConfiguration,
				Format: hap.FormatTLV8,
				Value:  val207,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeSelectedCameraRecordingConfiguration,
				Format: hap.FormatTLV8,
				Value:  val209,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   "226",
				Format: hap.FormatUInt8,
				Value:  0,
				Perms:  hap.EVPRPW,
			},
		},
	}
}

func ServiceDataStreamManagement() *hap.Service {
	val130, _ := tlv8.MarshalBase64(SupportedDataStreamTransportConfiguration{
		Configs: []TransferTransportConfiguration{
			{TransportType: 0}, // TCP
		},
	})

	return &hap.Service{
		Type: "129",
		Characters: []*hap.Character{
			{
				Type:   TypeSupportedDataStreamTransportConfiguration,
				Format: hap.FormatTLV8,
				Value:  val130,
				Perms:  hap.PR,
			},
			{
				Type:   TypeSetupDataStreamTransport,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  []string{"pr", "pw", "wr"},
			},
			{
				Type:   "37",
				Format: hap.FormatString,
				Value:  "1.0",
				Perms:  hap.PR,
			},
		},
	}
}

func ServiceDoorbell() *hap.Service {
	return &hap.Service{
		Type: "121",
		Characters: []*hap.Character{
			{
				Type:   "73",
				Format: hap.FormatUInt8,
				Value:  nil,
				Perms:  hap.EVPR,
			},
		},
	}
}
