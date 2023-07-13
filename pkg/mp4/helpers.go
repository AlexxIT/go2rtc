package mp4

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"strings"
)

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

func ParseCodecs(codecs string, parseAudio bool) (medias []*core.Media) {
	var videos []*core.Codec
	var audios []*core.Codec

	for _, name := range strings.Split(codecs, ",") {
		switch name {
		case MimeH264:
			codec := &core.Codec{Name: core.CodecH264}
			videos = append(videos, codec)
		case MimeH265:
			codec := &core.Codec{Name: core.CodecH265}
			videos = append(videos, codec)
		case MimeAAC:
			codec := &core.Codec{Name: core.CodecAAC}
			audios = append(audios, codec)
		case MimeFlac:
			audios = append(audios,
				&core.Codec{Name: core.CodecPCMA},
				&core.Codec{Name: core.CodecPCMU},
				&core.Codec{Name: core.CodecPCM},
			)
		case MimeOpus:
			codec := &core.Codec{Name: core.CodecOpus}
			audios = append(audios, codec)
		}
	}

	if videos != nil {
		media := &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs:    videos,
		}
		medias = append(medias, media)
	}

	if audios != nil && parseAudio {
		media := &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    audios,
		}
		medias = append(medias, media)
	}

	return
}

const (
	stateNone byte = iota
	stateInit
	stateStart
)
