package mp4

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
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
			&core.Codec{Name: core.CodecPCML},
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
				&core.Codec{Name: core.CodecPCML},
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

// PatchVideoRotate - update video track transformation matrix.
// Rotation supported by many players and browsers (except Safari).
// Scale has low support and better not to use it.
// Supported only 0, 90, 180, 270 degrees.
func PatchVideoRotate(init []byte, degrees int) bool {
	// search video atom
	i := bytes.Index(init, []byte("vide"))
	if i < 0 {
		return false
	}

	// seek to video matrix position
	i -= 4 + 3 + 1 + 8 + 32 + 8 + 4 + 4 + 4*9

	// Rotation matrix:
	// [   cos   sin     0]
	// [  -sin   cos     0]
	// [     0     0 16384]
	var cos, sin uint16

	switch degrees {
	case 0:
		cos = 1
		sin = 0
	case 90:
		cos = 0
		sin = 1
	case 180:
		cos = 0xFFFF // -1
		sin = 0
	case 270:
		cos = 0
		sin = 0xFFFF // -1
	default:
		return false
	}

	binary.BigEndian.PutUint16(init[i:], cos)
	binary.BigEndian.PutUint16(init[i+4:], sin)
	binary.BigEndian.PutUint16(init[i+12:], -sin)
	binary.BigEndian.PutUint16(init[i+16:], cos)

	return true
}

// PatchVideoScale - update "Pixel Aspect Ratio" atom.
// Supported by many players and browsers (except Firefox).
// Supported only positive integers.
func PatchVideoScale(init []byte, scaleX, scaleY int) bool {
	// search video atom
	i := bytes.Index(init, []byte("pasp"))
	if i < 0 {
		return false
	}

	binary.BigEndian.PutUint32(init[i+4:], uint32(scaleX))
	binary.BigEndian.PutUint32(init[i+8:], uint32(scaleY))

	return true
}
