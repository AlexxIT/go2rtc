package webp

import (
	"bytes"
	"image"
	"image/jpeg"

	webplib "github.com/skrashevich/go-webp"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/pion/rtp"
)

// EncodeImage encodes any image.Image to WebP lossy bytes.
func EncodeImage(img image.Image, quality int) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := webplib.Encode(buf, img, &webplib.Options{Lossy: true, Quality: float32(quality)}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeLossless encodes image.Image to WebP lossless bytes.
func EncodeLossless(img image.Image) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := webplib.Encode(buf, img, &webplib.Options{Lossy: false}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeJPEG converts JPEG bytes to WebP lossy bytes.
func EncodeJPEG(jpegData []byte, quality int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	return EncodeImage(img, quality)
}

// Decode decodes WebP bytes to image.Image.
func Decode(data []byte) (image.Image, error) {
	return webplib.Decode(bytes.NewReader(data))
}

// FixJPEGToWebP is like mjpeg.FixJPEG but outputs WebP. Handles AVI1 MJPEG frames.
func FixJPEGToWebP(jpegData []byte, quality int) ([]byte, error) {
	fixed := mjpeg.FixJPEG(jpegData)
	return EncodeJPEG(fixed, quality)
}

// Encoder converts a RAW YUV frame to WebP.
func Encoder(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	newImage := y4m.NewImage(codec.FmtpLine)

	return func(packet *rtp.Packet) {
		img := newImage(packet.Payload)

		b, err := EncodeImage(img, 75)
		if err != nil {
			return
		}

		clone := *packet
		clone.Payload = b
		handler(&clone)
	}
}

// JPEGToWebP converts a JPEG frame packet to WebP.
func JPEGToWebP(handler core.HandlerFunc) core.HandlerFunc {
	return func(packet *rtp.Packet) {
		b, err := EncodeJPEG(packet.Payload, 75)
		if err != nil {
			return
		}

		clone := *packet
		clone.Payload = b
		handler(&clone)
	}
}
