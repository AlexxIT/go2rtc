package iso

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestWriteVideoH265UsesHvc1SampleEntry(t *testing.T) {
	const config = "h265 config"

	m := NewMovie(128)
	m.WriteVideo(core.CodecH265, 1920, 1080, []byte(config))

	if bytes.Contains(m.Bytes(), []byte("hev1")) {
		t.Fatalf("H265 video sample entry should not use hev1: %q", m.Bytes())
	}
	if !bytes.Contains(m.Bytes(), []byte("hvc1")) {
		t.Fatalf("H265 video sample entry should use hvc1: %q", m.Bytes())
	}

	atom, err := DecodeAtom(m.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	video, ok := atom.(*AtomVideo)
	if !ok {
		t.Fatalf("expected AtomVideo, got %T", atom)
	}
	if video.Name != "hvc1" {
		t.Fatalf("expected hvc1 sample entry, got %q", video.Name)
	}
	if string(video.Config) != config {
		t.Fatalf("expected config %q, got %q", config, video.Config)
	}
}
