package ascii

import (
	"image"
	"testing"
)

func TestXterm256Color(t *testing.T) {
	tests := []struct {
		r, g, b uint8
		n       int
		want    uint8
	}{
		{255, 0, 0, 6, 1},
		{0, 255, 0, 6, 2},
		{0, 0, 255, 6, 4},
		{255, 255, 255, 6, 3},
		{0, 0, 0, 6, 0},
	}

	for _, tt := range tests {
		got := xterm256color(tt.r, tt.g, tt.b, tt.n)
		if got != tt.want {
			t.Errorf("xterm256color(%v, %v, %v, %v) = %v; want %v", tt.r, tt.g, tt.b, tt.n, got, tt.want)
		}
	}
}

func TestResizeImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	newImg := resizeImage(img, 50, 50)

	if newImg.Bounds().Dx() != 50 || newImg.Bounds().Dy() != 50 {
		t.Errorf("resizeImage: expected dimensions (50, 50), got (%d, %d)", newImg.Bounds().Dx(), newImg.Bounds().Dy())
	}

	newImg = resizeImage(img, 0, 50)
	if newImg.Bounds().Dx() != 50 || newImg.Bounds().Dy() != 50 {
		t.Errorf("resizeImage: expected dimensions (50, 50), got (%d, %d)", newImg.Bounds().Dx(), newImg.Bounds().Dy())
	}

	newImg = resizeImage(img, 50, 0)
	if newImg.Bounds().Dx() != 50 || newImg.Bounds().Dy() != 50 {
		t.Errorf("resizeImage: expected dimensions (50, 50), got (%d, %d)", newImg.Bounds().Dx(), newImg.Bounds().Dy())
	}
}

func BenchmarkXterm256Color(b *testing.B) {
	for i := 0; i < b.N; i++ {
		xterm256color(255, 0, 0, 6)
	}
}

func BenchmarkResizeImage(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	for i := 0; i < b.N; i++ {
		resizeImage(img, 500, 500)
	}
}
