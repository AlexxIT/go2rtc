package mp4

import "github.com/AlexxIT/go2rtc/pkg/core"

// ParseQuery - like usual parse, but with mp4 param handler
func ParseQuery(query map[string][]string) []*core.Media {
	if v := query["mp4"]; len(v) != 0 {
		medias := []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecH264},
					{Name: core.CodecH265},
				},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecAAC},
				},
			},
		}

		if v[0] == "" {
			return medias // legacy
		}

		medias[1].Codecs = append(medias[1].Codecs,
			&core.Codec{Name: core.CodecPCMA},
			&core.Codec{Name: core.CodecPCMU},
			&core.Codec{Name: core.CodecPCM},
		)

		if v[0] == "flac" {
			return medias // modern browsers
		}

		medias[1].Codecs = append(medias[1].Codecs,
			&core.Codec{Name: core.CodecOpus},
			&core.Codec{Name: core.CodecMP3},
		)

		return medias // Chrome, FFmpeg, VLC
	}

	return core.ParseQuery(query)
}

const (
	waitNone byte = iota
	waitKeyframe
	waitInit
)
