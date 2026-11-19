package webrtc

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// h264Media mirrors a video media parsed from a remote SDP answer that
// lists several H264 payload types.
func h264Media() (*core.Media, *core.Codec, *core.Codec) {
	pt96 := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96, FmtpLine: "profile-level-id=42001f"}
	pt98 := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 98, FmtpLine: "profile-level-id=640032"}
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{pt96, pt98}}
	return media, pt96, pt98
}

// A consumer that dials the connection binds a receiver to the first H264
// codec before any track has arrived. When the remote then sends on a
// different payload type, OnTrack must feed that receiver rather than create
// a second one the consumer is not attached to.
func TestReceiverForReusesEarlyBoundReceiver(t *testing.T) {
	c := &Conn{Mode: core.ModeActiveProducer} // how nest dials
	media, offered, actual := h264Media()

	early, err := c.GetTrack(media, offered)
	require.NoError(t, err)
	require.Len(t, c.Receivers, 1)

	got, err := c.receiverFor(media, actual)
	require.NoError(t, err)
	require.Same(t, early, got, "the early-bound receiver must be reused")
	require.Same(t, actual, got.Codec, "its codec must be the one actually in use")
	require.Len(t, c.Receivers, 1, "no second receiver")

	// a later consumer, attaching after OnTrack, resolves the same receiver
	later, err := c.GetTrack(media, actual)
	require.NoError(t, err)
	require.Same(t, early, later)
}

// The exact codec pointer still wins when it exists.
func TestReceiverForPrefersExactCodec(t *testing.T) {
	c := &Conn{Mode: core.ModeActiveProducer} // how nest dials
	media, offered, actual := h264Media()

	r1, err := c.GetTrack(media, offered)
	require.NoError(t, err)
	r2, err := c.GetTrack(media, actual)
	require.NoError(t, err)
	require.NotSame(t, r1, r2)

	got, err := c.receiverFor(media, actual)
	require.NoError(t, err)
	require.Same(t, r2, got)
}

// A receiver that is already receiving packets belongs to a live track and
// must not be repointed; and a receiver on another media is never a match.
func TestReceiverForDoesNotHijack(t *testing.T) {
	c := &Conn{Mode: core.ModeActiveProducer} // how nest dials
	media, offered, actual := h264Media()

	live, err := c.GetTrack(media, offered)
	require.NoError(t, err)
	live.Packets = 1

	got, err := c.receiverFor(media, actual)
	require.NoError(t, err)
	require.NotSame(t, live, got, "a receiving receiver must keep its codec")
	require.Same(t, offered, live.Codec)
	require.Len(t, c.Receivers, 2)

	audio := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}
	opus := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: 111}
	idleAudio, err := c.GetTrack(audio, opus)
	require.NoError(t, err)

	other := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 102}
	got, err = c.receiverFor(media, other)
	require.NoError(t, err)
	require.NotSame(t, idleAudio, got, "a receiver on another media is not a candidate")
}
