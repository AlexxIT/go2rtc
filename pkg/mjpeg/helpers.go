package mjpeg

import (
	"bytes"
	"image/jpeg"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/pion/rtp"
)

func FixJPEG(b []byte) []byte {
	// skip non-JPEG
	if len(b) < 10 || b[0] != 0xFF || b[1] != markerSOI {
		return b
	}

	// skip JPEG without app marker
	if b[2] == 0xFF && b[3] == markerDQT {
		return b
	}

	switch string(b[6:10]) {
	case "JFIF", "Exif":
		// skip if header OK for imghdr library
		// - https://docs.python.org/3/library/imghdr.html
		return b
	case "AVI1":
		// adds DHT tables to JPEG file before SOS marker
		// useful when you want to save a JPEG frame from an MJPEG stream
		// - https://github.com/image-rs/jpeg-decoder/issues/76
		// - https://github.com/pion/mediadevices/pull/493
		// - https://bugzilla.mozilla.org/show_bug.cgi?id=963907#c18
		return InjectDHT(b)
	}

	// reencode JPEG if it has wrong header
	//
	// for example, this app produce "bad" images:
	// https://github.com/jacksonliam/mjpg-streamer
	//
	// and they can't be uploaded to the Telegram servers:
	// {"ok":false,"error_code":400,"description":"Bad Request: IMAGE_PROCESS_FAILED"}
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

// Encoder convert YUV frame to Img.
// Support skipping empty frames, for example if USB cam needs time to start.
func Encoder(codec *core.Codec, skipEmpty int, handler core.HandlerFunc) core.HandlerFunc {
	newImage := y4m.NewImage(codec.FmtpLine)

	return func(packet *rtp.Packet) {
		img := newImage(packet.Payload)

		if skipEmpty != 0 && y4m.HasSameColor(img) {
			skipEmpty--
			return
		}

		buf := bytes.NewBuffer(nil)
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return
		}

		clone := *packet
		clone.Payload = buf.Bytes()
		handler(&clone)
	}
}

const dhtSize = 432 // known size for 4 default tables

func InjectDHT(b []byte) []byte {
	if bytes.Index(b, []byte{0xFF, markerDHT}) > 0 {
		return b // already exist
	}

	i := bytes.Index(b, []byte{0xFF, markerSOS})
	if i < 0 {
		return b
	}

	dht := make([]byte, 0, dhtSize)
	dht = MakeHuffmanHeaders(dht)

	tmp := make([]byte, len(b)+dhtSize)
	copy(tmp, b[:i])
	copy(tmp[i:], dht)
	copy(tmp[i+dhtSize:], b[i:])

	return tmp
}
