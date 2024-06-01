package helpers

import (
	"reflect"
	"testing"
)

func TestParseFFmpegEncoders(t *testing.T) {
	input := `
 V..... h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 V..... hevc_nvenc           NVIDIA NVENC hevc encoder (codec hevc)
 VFS..D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 A....D aac                  AAC (Advanced Audio Coding) (codec aac)
`

	expected := []Encoder{
		{Type: "V", FrameLevelMT: false, SliceLevelMT: false, Experimental: false, DrawHorizBand: false, DirectRender: false, Name: "h264_nvenc", Description: "NVIDIA NVENC H.264 encoder (codec h264)"},
		{Type: "V", FrameLevelMT: false, SliceLevelMT: false, Experimental: false, DrawHorizBand: false, DirectRender: false, Name: "hevc_nvenc", Description: "NVIDIA NVENC hevc encoder (codec hevc)"},
		{Type: "V", FrameLevelMT: true, SliceLevelMT: true, Experimental: false, DrawHorizBand: false, DirectRender: true, Name: "libx264", Description: "libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)"},
		{Type: "A", FrameLevelMT: false, SliceLevelMT: false, Experimental: false, DrawHorizBand: false, DirectRender: true, Name: "aac", Description: "AAC (Advanced Audio Coding) (codec aac)"},
	}

	result := ParseFFmpegEncoders(input)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ParseFFmpegEncoders() = %v, want %v", result, expected)
	}
}

func TestIsEncoderSupported(t *testing.T) {
	Encoders = []Encoder{
		{Name: "h264_nvenc"},
		{Name: "hevc_nvenc"},
		{Name: "libx264"},
		{Name: "aac"},
	}

	tests := []struct {
		codec   string
		support bool
	}{
		{"h264_nvenc", true},
		{"hevc_nvenc", true},
		{"libx264", true},
		{"aac", true},
		{"nonexistent_codec", false},
	}

	for _, tt := range tests {
		t.Run(tt.codec, func(t *testing.T) {
			if got := IsEncoderSupported(tt.codec); got != tt.support {
				t.Errorf("IsEncoderSupported(%s) = %v, want %v", tt.codec, got, tt.support)
			}
		})
	}
}
