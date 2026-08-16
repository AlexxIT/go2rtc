package dahua

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Dahua wraps every media payload in a DHAV "sub frame". The 8 byte audio
// variant is the one used for two way audio (talk).
//
//	[0:3] 00 00 01     start code
//	[3]   F0           frame type: audio
//	[4]   codec id     see dhavAudioCodecID
//	[5]   rate index   see dhavSampleRates
//	[6:8] payload len  uint16 little endian
//
// Sources (the talk protocol was confirmed by a live capture of the prebuilt
// Talk.exe driving a Dahua E4702, then cross-checked against two references):
//
//   - FFmpeg libavformat/dhav.c: frame type 0xF0 = audio, plus the codec id and
//     sample-rate tables below, copied verbatim. (The 00 00 01 sub-frame start
//     code is the talk-specific prefix seen in the live Talk.exe capture.)
//   - Official C Win64 NetSDK V3.061, dhnetsdk.h: DH_TALK_CODING_TYPE supplies
//     the EncodeFormat coding-type values (0 DEFAULT, 1 PCM, 2 G711a, 4 G711u).
//   - Official NetSDK Android demo, TalkModule.java -> AudioRecord(): builds the
//     same header — cross-check only; the Android SDK is not an authority here.
const (
	dhavTypeAudio = 0xF0
)

// dhavSampleRates is the sample rate index table (FFmpeg libavformat/dhav.c).
// Note index 0 and 2 are both 8000; Dahua's own client emits 2 for 8 kHz.
var dhavSampleRates = []uint32{
	8000, 4000, 8000, 11025, 16000,
	20000, 22050, 32000, 44100, 48000,
	96000, 192000, 64000,
}

// dhavSampleRateIndex returns the wire index for a sample rate, defaulting to
// the 8 kHz index Dahua clients use.
func dhavSampleRateIndex(rate uint32) byte {
	if rate == 8000 {
		return 2 // what the official demo sends
	}
	for i, r := range dhavSampleRates {
		if r == rate {
			return byte(i)
		}
	}
	return 2
}

// dhavAudioCodecID maps a go2rtc codec to the Dahua wire codec id.
// Values from FFmpeg libavformat/dhav.c dhav_read_packet().
func dhavAudioCodecID(name string) (byte, bool) {
	switch name {
	case core.CodecPCMA:
		return dhavCodecG711A, true // 0x0E, the E4702's native talk format
	case core.CodecPCMU:
		return dhavCodecG711U, true // 0x0A; 0x16 also seen in the wild
	case core.CodecPCML:
		return dhavCodecPCM16, true // 0x0C, PCM16 — confirmed rendered on E4702 hardware
	case core.CodecAAC:
		return 0x1A, true
	}
	return 0, false
}

// frameBytes returns the payload size for one packet period, e.g. 320 bytes
// for 40 ms of 8 kHz G.711 - the chunk size Dahua's own clients use.
func frameBytes(codec *core.Codec, millis int) int {
	if millis <= 0 {
		millis = 40
	}
	rate := codec.ClockRate
	if rate == 0 {
		rate = 8000
	}

	var bytesPerSample int
	switch codec.Name {
	case core.CodecPCMA, core.CodecPCMU:
		bytesPerSample = 1
	case core.CodecPCML:
		bytesPerSample = 2
	default:
		return 0 // compressed codecs keep their native frame size
	}

	n := int(rate) * millis / 1000 * bytesPerSample
	if n <= 0 {
		return 320
	}
	return n
}
