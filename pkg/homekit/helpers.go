package homekit

import (
	"encoding/hex"
	"slices"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
)

var videoCodecs = [...]string{core.CodecH264}
var videoProfiles = [...]string{"4200", "4D00", "6400"}
var videoLevels = [...]string{"1F", "20", "28"}

func videoToMedia(codecs []camera.VideoCodec) *core.Media {
	media := &core.Media{
		Kind: core.KindVideo, Direction: core.DirectionRecvonly,
	}

	for _, codec := range codecs {
		for _, param := range codec.CodecParams {
			// get best profile and level
			profileID := slices.Max(param.ProfileID)
			level := slices.Max(param.Level)
			profile := videoProfiles[profileID] + videoLevels[level]
			mediaCodec := &core.Codec{
				Name:      videoCodecs[codec.CodecType],
				ClockRate: 90000,
				FmtpLine:  "profile-level-id=" + profile,
			}
			media.Codecs = append(media.Codecs, mediaCodec)
		}
	}

	return media
}

var audioCodecs = [...]string{core.CodecPCMU, core.CodecPCMA, core.CodecELD, core.CodecOpus}
var audioSampleRates = [...]uint32{8000, 16000, 24000}

func audioToMedia(codecs []camera.AudioCodec) *core.Media {
	media := &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionRecvonly,
	}

	for _, codec := range codecs {
		for _, param := range codec.CodecParams {
			for _, sampleRate := range param.SampleRate {
				mediaCodec := &core.Codec{
					Name:      audioCodecs[codec.CodecType],
					ClockRate: audioSampleRates[sampleRate],
					Channels:  uint16(param.Channels),
				}

				if mediaCodec.Name == core.CodecELD {
					// only this version works with FFmpeg
					conf := aac.EncodeConfig(aac.TypeAACELD, 24000, 1, true)
					mediaCodec.FmtpLine = aac.FMTP + hex.EncodeToString(conf)
				}

				media.Codecs = append(media.Codecs, mediaCodec)
			}
		}
	}

	return media
}

func trackToVideo(track *core.Receiver, video0 *camera.VideoCodec) *camera.VideoCodec {
	profileID := video0.CodecParams[0].ProfileID[0]
	level := video0.CodecParams[0].Level[0]
	attrs := video0.VideoAttrs[0]

	if track != nil {
		profile := h264.GetProfileLevelID(track.Codec.FmtpLine)

		for i, s := range videoProfiles {
			if s == profile[:4] {
				profileID = byte(i)
				break
			}
		}

		for i, s := range videoLevels {
			if s == profile[4:] {
				level = byte(i)
				break
			}
		}

		for _, s := range video0.VideoAttrs {
			if s.Width > attrs.Width || s.Height > attrs.Height {
				attrs = s
			}
		}
	}

	return &camera.VideoCodec{
		CodecType: video0.CodecType,
		CodecParams: []camera.VideoParams{
			{
				ProfileID: []byte{profileID},
				Level:     []byte{level},
			},
		},
		VideoAttrs: []camera.VideoAttrs{attrs},
	}
}

func trackToAudio(track *core.Receiver, audio0 *camera.AudioCodec) *camera.AudioCodec {
	codecType := audio0.CodecType
	channels := audio0.CodecParams[0].Channels
	sampleRate := audio0.CodecParams[0].SampleRate[0]

	if track != nil {
		channels = uint8(track.Codec.Channels)

		for i, s := range audioCodecs {
			if s == track.Codec.Name {
				codecType = byte(i)
				break
			}
		}

		for i, s := range audioSampleRates {
			if s == track.Codec.ClockRate {
				sampleRate = byte(i)
				break
			}
		}
	}

	return &camera.AudioCodec{
		CodecType: codecType,
		CodecParams: []camera.AudioParams{
			{
				Channels:   channels,
				SampleRate: []byte{sampleRate},
				RTPTime:    []uint8{20},
			},
		},
	}
}
