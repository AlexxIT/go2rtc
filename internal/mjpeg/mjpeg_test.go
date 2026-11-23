package mjpeg

import (
	"net/url"
	"testing"
)

func TestFrameCacheKey(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
		want  string
	}{
		{
			name:  "includes source and output options",
			query: url.Values{"src": {"camera"}, "width": {"640"}, "height": {"480"}, "rotate": {"90"}},
			want:  "height=480&rotate=90&src=camera&width=640",
		},
		{
			name:  "includes short output option names",
			query: url.Values{"src": {"camera"}, "w": {"640"}, "h": {"480"}},
			want:  "h=480&src=camera&w=640",
		},
		{
			name:  "excludes cache lifetime and unrelated options",
			query: url.Values{"src": {"camera"}, "width": {"640"}, "cache": {"1s"}, "quality": {"80"}},
			want:  "src=camera&width=640",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheKey(tt.query); got != tt.want {
				t.Fatalf("frameCacheKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFrameCacheKeySeparatesOutputVariants(t *testing.T) {
	base := cacheKey(url.Values{"src": {"camera"}})
	width := cacheKey(url.Values{"src": {"camera"}, "width": {"640"}})
	rotate := cacheKey(url.Values{"src": {"camera"}, "rotate": {"90"}})
	otherSource := cacheKey(url.Values{"src": {"other-camera"}})

	if base == width || base == rotate || base == otherSource {
		t.Fatalf("different frame requests must have different cache keys: base=%q width=%q rotate=%q source=%q", base, width, rotate, otherSource)
	}
}

func TestFrameCacheKeyIgnoresCacheLifetime(t *testing.T) {
	short := cacheKey(url.Values{"src": {"camera"}, "cache": {"1s"}})
	long := cacheKey(url.Values{"src": {"camera"}, "cache": {"1m"}})

	if short != long {
		t.Fatalf("cache lifetime must not change the frame cache key: short=%q long=%q", short, long)
	}
}
