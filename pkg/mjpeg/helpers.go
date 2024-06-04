package mjpeg

import (
	"bytes"
	"image/jpeg"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/pion/rtp"
)

// FixJPEG - reencode JPEG if it has wrong header
//
// for example, this app produce "bad" images:
// https://github.com/jacksonliam/mjpg-streamer
//
// and they can't be uploaded to the Telegram servers:
// {"ok":false,"error_code":400,"description":"Bad Request: IMAGE_PROCESS_FAILED"}
func FixJPEG(b []byte) []byte {
	// skip non-JPEG
	if len(b) < 10 || b[0] != 0xFF || b[1] != 0xD8 {
		return b
	}
	// skip if header OK for imghdr library
	// https://docs.python.org/3/library/imghdr.html
	if string(b[2:4]) == "\xFF\xDB" || string(b[6:10]) == "JFIF" || string(b[6:10]) == "Exif" {
		return b
	}

	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, nil); err != nil {
		return b
	}
	return buf.Bytes()
}

func Encoder(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	newImage := y4m.NewImage(codec.FmtpLine)

	return func(packet *rtp.Packet) {
		img := newImage(packet.Payload)

		buf := bytes.NewBuffer(nil)
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return
		}

		clone := *packet
		clone.Payload = buf.Bytes()
		handler(&clone)
	}
}
