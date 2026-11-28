package webrtc

import (
	"sync"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestGetTrackReusesReceiverForCompatibleCodec(t *testing.T) {
	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
	}
	firstCodec := &core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		PayloadType: 96,
		FmtpLine:    "packetization-mode=1;profile-level-id=42e01f",
	}
	transmittedCodec := &core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		PayloadType: 98,
		FmtpLine:    "packetization-mode=1;profile-level-id=64001f",
	}
	media.Codecs = []*core.Codec{firstCodec, transmittedCodec}

	conn := &Conn{Mode: core.ModeActiveProducer}

	receiver, err := conn.GetTrack(media, firstCodec)
	require.NoError(t, err)

	reused, err := conn.GetTrack(media, transmittedCodec)
	require.NoError(t, err)

	require.Same(t, receiver, reused)
	require.Len(t, conn.Receivers, 1)
	require.Same(t, transmittedCodec, conn.Receivers[0].Codec)
}

func TestGetTrackCreatesReceiverForIncompatibleCodec(t *testing.T) {
	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
	}
	h264 := &core.Codec{
		Name:      core.CodecH264,
		ClockRate: 90000,
	}
	vp8 := &core.Codec{
		Name:      core.CodecVP8,
		ClockRate: 90000,
	}
	media.Codecs = []*core.Codec{h264, vp8}

	conn := &Conn{Mode: core.ModeActiveProducer}

	first, err := conn.GetTrack(media, h264)
	require.NoError(t, err)

	second, err := conn.GetTrack(media, vp8)
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Len(t, conn.Receivers, 2)
	require.Same(t, vp8, conn.Receivers[1].Codec)
}

func TestGetTrackConcurrent(t *testing.T) {
	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
	}

	codecs := []*core.Codec{
		{
			Name:        core.CodecH264,
			ClockRate:   90000,
			PayloadType: 96,
			FmtpLine:    "packetization-mode=1;profile-level-id=42e01f",
		},
		{
			Name:        core.CodecH264,
			ClockRate:   90000,
			PayloadType: 98,
			FmtpLine:    "packetization-mode=1;profile-level-id=64001f",
		},
	}
	media.Codecs = codecs

	for _, mode := range []core.Mode{
		core.ModeActiveProducer,
		core.ModePassiveProducer,
	} {
		conn := &Conn{Mode: mode}

		const workers = 100

		tracks := make([]*core.Receiver, workers)
		errs := make([]error, workers)

		start := make(chan struct{})
		var wg sync.WaitGroup

		// Call GetTrack concurrently with different compatible codecs.
		wg.Add(workers)
		for i := range workers {
			go func() {
				defer wg.Done()
				<-start

				tracks[i], errs[i] = conn.GetTrack(media, codecs[i%2])
			}()
		}

		close(start)
		wg.Wait()

		// All calls must return the same receiver.
		require.Len(t, conn.Receivers, 1)

		for i, track := range tracks {
			require.NoError(t, errs[i])
			require.Same(t, conn.Receivers[0], track)
		}
	}
}
