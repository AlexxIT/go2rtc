package webp

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func newTestImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return img
}

func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		bytes.Equal(data[0:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP"))
}

func TestEncodeImage(t *testing.T) {
	img := newTestImage(100, 100)
	data, err := EncodeImage(img, 75)
	if err != nil {
		t.Fatalf("EncodeImage error: %v", err)
	}
	if !isWebP(data) {
		t.Fatalf("output is not valid WebP: got prefix %q", data[:min(12, len(data))])
	}
}

func TestEncodeJPEG(t *testing.T) {
	img := newTestImage(100, 100)
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg.Encode error: %v", err)
	}
	data, err := EncodeJPEG(jpegBuf.Bytes(), 75)
	if err != nil {
		t.Fatalf("EncodeJPEG error: %v", err)
	}
	if !isWebP(data) {
		t.Fatalf("output is not valid WebP: got prefix %q", data[:min(12, len(data))])
	}
}

func TestDecode(t *testing.T) {
	img := newTestImage(100, 80)
	data, err := EncodeImage(img, 80)
	if err != nil {
		t.Fatalf("EncodeImage error: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 80 {
		t.Fatalf("expected 100x80, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestRoundTrip(t *testing.T) {
	img := newTestImage(64, 64)
	data, err := EncodeLossless(img)
	if err != nil {
		t.Fatalf("EncodeLossless error: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			orig := img.At(x, y)
			got := decoded.At(x, y)
			or, og, ob, oa := orig.RGBA()
			gr, gg, gb, ga := got.RGBA()
			if or != gr || og != gg || ob != gb || oa != ga {
				t.Fatalf("pixel mismatch at (%d,%d): want %v got %v", x, y, orig, got)
			}
		}
	}
}

func TestEncodeLossless(t *testing.T) {
	img := newTestImage(50, 50)
	data, err := EncodeLossless(img)
	if err != nil {
		t.Fatalf("EncodeLossless error: %v", err)
	}
	if !isWebP(data) {
		t.Fatalf("output is not valid WebP")
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			orig := img.At(x, y)
			got := decoded.At(x, y)
			or, og, ob, oa := orig.RGBA()
			gr, gg, gb, ga := got.RGBA()
			if or != gr || og != gg || ob != gb || oa != ga {
				t.Fatalf("pixel mismatch at (%d,%d): want %v got %v", x, y, orig, got)
			}
		}
	}
}

func TestNewConsumer(t *testing.T) {
	c := NewConsumer()
	if c == nil {
		t.Fatal("NewConsumer returned nil")
	}
	if c.FormatName != "webp" {
		t.Fatalf("expected FormatName=webp, got %q", c.FormatName)
	}
	if len(c.Medias) == 0 {
		t.Fatal("expected at least one media")
	}
	media := c.Medias[0]
	if media.Kind != core.KindVideo {
		t.Fatalf("expected KindVideo, got %v", media.Kind)
	}
	if media.Direction != core.DirectionSendonly {
		t.Fatalf("expected DirectionSendonly, got %v", media.Direction)
	}
	hasJPEG := false
	hasRAW := false
	for _, codec := range media.Codecs {
		if codec.Name == core.CodecJPEG {
			hasJPEG = true
		}
		if codec.Name == core.CodecRAW {
			hasRAW = true
		}
	}
	if !hasJPEG {
		t.Fatal("expected JPEG codec in consumer medias")
	}
	if !hasRAW {
		t.Fatal("expected RAW codec in consumer medias")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
