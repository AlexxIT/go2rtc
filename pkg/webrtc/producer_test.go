package webrtc

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// A Nest video m-line is negotiated with two H264 payload types. The consumer
// and the incoming RTP track must bind to the same receiver, or no video is
// forwarded (observed as a 27-minute dead stream on a real Nest camera).
func TestGetTrackReusesReceiverPerMedia(t *testing.T) {
	codecA := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}
	codecB := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 102}
	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codecA, codecB},
	}

	c := &Conn{Mode: core.ModeActiveProducer}
	c.Medias = []*core.Media{media}

	r1, err := c.GetTrack(media, codecA)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := c.GetTrack(media, codecB)
	if err != nil {
		t.Fatal(err)
	}
	if r1 != r2 {
		t.Fatalf("expected the same receiver for both codecs of one media, got %p and %p", r1, r2)
	}
	if len(c.Receivers) != 1 {
		t.Fatalf("expected 1 receiver, got %d", len(c.Receivers))
	}
}
