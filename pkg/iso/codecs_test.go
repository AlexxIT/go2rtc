package iso

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestWriteVideoH265UsesHVC1(t *testing.T) {
	config := []byte{1, 2, 3}
	movie := NewMovie(128)
	movie.WriteVideo(core.CodecH265, 1920, 1080, config)

	data := movie.Bytes()
	if got := string(data[4:8]); got != "hvc1" {
		t.Fatalf("expected hvc1 sample entry, got %q", got)
	}
	if bytes.Contains(data, []byte("hev1")) {
		t.Fatal("H.265 sample entry must not use hev1 when the MIME codec is hvc1")
	}

	atom, err := DecodeAtom(data)
	if err != nil {
		t.Fatal(err)
	}
	video, ok := atom.(*AtomVideo)
	if !ok {
		t.Fatalf("expected AtomVideo, got %T", atom)
	}
	if video.Name != "hvc1" {
		t.Fatalf("expected decoded hvc1 sample entry, got %q", video.Name)
	}
	if !bytes.Equal(video.Config, config) {
		t.Fatalf("expected config %v, got %v", config, video.Config)
	}
}
