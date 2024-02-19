package device

import (
	"net/url"
	"testing"
)

func TestQueryToInput(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
		want  string
	}{
		{
			name: "video with resolution",
			query: url.Values{
				"video":      {"example_video"},
				"resolution": {"1920x1080"},
			},
			want: "-f v4l2 -video_size 1920x1080 -i example_video",
		},
		{
			name: "video with video_size",
			query: url.Values{
				"video":      {"example_video"},
				"video_size": {"1280x720"},
			},
			want: "-f v4l2 -video_size 1280x720 -i example_video",
		},
		{
			name: "video with multiple options",
			query: url.Values{
				"video":        {"example_video"},
				"video_size":   {"1280x720"},
				"pixel_format": {"yuv420p"},
				"framerate":    {"30"},
			},
			want: "-f v4l2 -video_size 1280x720 -pixel_format yuv420p -framerate 30 -i example_video",
		},
		{
			name: "audio",
			query: url.Values{
				"audio": {"example_audio"},
			},
			want: "-f alsa -i example_audio",
		},
		{
			name:  "empty query",
			query: url.Values{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryToInput(tt.query); got != tt.want {
				t.Errorf("queryToInput() = %v, want %v", got, tt.want)
			}
		})
	}
}
