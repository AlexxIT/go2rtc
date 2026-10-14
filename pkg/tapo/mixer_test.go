package tapo

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func pcma(v int16, n int) []byte {
	return bytes.Repeat([]byte{pcm.PCMtoPCMA(v)}, n)
}

func TestMixPCMAOneSource(t *testing.T) {
	part := pcma(1000, mixerChunkSize)
	require.Equal(t, part, mixPCMA([][]byte{part}, mixerChunkSize))
}

func TestMixPCMAShortPart(t *testing.T) {
	chunk := mixPCMA([][]byte{pcma(1000, 100)}, mixerChunkSize)
	require.Len(t, chunk, mixerChunkSize)
	require.Equal(t, pcma(1000, 100), chunk[:100])
	require.Equal(t, bytes.Repeat([]byte{pcmaSilence}, mixerChunkSize-100), chunk[100:])
}

func TestMixPCMATwoSources(t *testing.T) {
	chunk := mixPCMA([][]byte{pcma(4000, mixerChunkSize), pcma(2000, mixerChunkSize)}, mixerChunkSize)
	// A-law is lossy, the sum has to be close to 6000
	require.InDelta(t, 6000, int(pcm.PCMAtoPCM(chunk[0])), 300)
	require.InDelta(t, 6000, int(pcm.PCMAtoPCM(chunk[mixerChunkSize-1])), 300)

	// opposite signals cancel each other
	chunk = mixPCMA([][]byte{pcma(4000, mixerChunkSize), pcma(-4000, mixerChunkSize)}, mixerChunkSize)
	require.InDelta(t, 0, int(pcm.PCMAtoPCM(chunk[0])), 300)
}

func TestLimitPCM(t *testing.T) {
	// no change below the knee
	require.Equal(t, int16(1234), limitPCM(1234, 2))
	require.Equal(t, int16(-mixerKnee), limitPCM(-mixerKnee, 3))

	// the largest sum fits and the curve is monotonic
	for _, n := range []int{2, 3, 4, 9, 16} {
		peak := int32(n) * 32768
		require.LessOrEqual(t, int(limitPCM(peak, n)), 32767)
		require.GreaterOrEqual(t, int(limitPCM(-peak, n)), -32767)

		prev := int16(0)
		for v := int32(0); v <= peak; v += 97 {
			cur := limitPCM(v, n)
			require.GreaterOrEqual(t, cur, prev)
			prev = cur
		}
	}
}

func TestMixerNext(t *testing.T) {
	m := newMixer(mixerOptions{}, nil, nil)
	a, b := m.AddSource(), m.AddSource()

	a.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize+100)})
	b.Push(&rtp.Packet{Payload: pcma(1000, 50)})

	// only full chunks without flush
	chunk, n := m.next(false)
	require.Len(t, chunk, mixerChunkSize)
	require.Equal(t, 1, n)

	chunk, n = m.next(false)
	require.Nil(t, chunk)
	require.Equal(t, 0, n)

	// the rest of both sources with flush
	chunk, n = m.next(true)
	require.Len(t, chunk, mixerChunkSize)
	require.Equal(t, 2, n)
	require.Empty(t, a.queue)
	require.Empty(t, b.queue)
}

func TestMixerClear(t *testing.T) {
	m := newMixer(mixerOptions{}, nil, nil)
	src := m.AddSource()
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})

	m.Clear()
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})
	require.Empty(t, src.queue)

	// a new source is not muted
	other := m.AddSource()
	other.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})
	require.Len(t, other.queue, mixerChunkSize)

	// a live source is heard again after the mute
	src.muted = time.Now().Add(-time.Millisecond)
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})
	require.Len(t, src.queue, mixerChunkSize)
}

func TestMixerFinished(t *testing.T) {
	m := newMixer(mixerOptions{}, func([]byte) error {
		return errors.New("broken pipe")
	}, nil)
	m.start(0, func() error { return nil })
	require.False(t, m.Finished())

	// the first write fails, the sender loop returns
	src := m.AddSource()
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})
	require.Eventually(t, m.Finished, 2*time.Second, 10*time.Millisecond)

	// a setup error also ends the mixer
	m = newMixer(mixerOptions{}, nil, nil)
	m.start(0, func() error { return errors.New("busy") })
	require.Eventually(t, m.Finished, time.Second, 10*time.Millisecond)
}

func TestMixerRun(t *testing.T) {
	var mu sync.Mutex
	var chunks [][]byte
	var stats mixerStats

	opts := mixerOptions{prefillChunks: 2, leadChunks: 3}
	m := newMixer(opts, func(chunk []byte) error {
		mu.Lock()
		chunks = append(chunks, chunk)
		mu.Unlock()
		return nil
	}, func(event string, s *mixerStats) {
		if s != nil {
			mu.Lock()
			stats = *s
			mu.Unlock()
		}
	})
	m.start(0, func() error { return nil })

	// 5.5 chunks of audio in packets of 24 ms, as FFmpeg sends an MP3
	audio := pcma(1000, mixerChunkSize*5+mixerChunkSize/2)
	src := m.AddSource()
	for len(audio) > 0 {
		n := 192
		if n > len(audio) {
			n = len(audio)
		}
		src.Push(&rtp.Packet{Payload: audio[:n]})
		audio = audio[n:]
	}

	time.Sleep(mixerChunkTime * 5)
	m.Stop(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// pre-fill, lead-in, 6 chunks of audio (the last one filled up), tail
	require.Len(t, chunks, 2+3+6+2)
	for _, chunk := range chunks {
		require.Len(t, chunk, mixerChunkSize)
	}
	silence := bytes.Repeat([]byte{pcmaSilence}, mixerChunkSize)
	require.Equal(t, silence, chunks[4])
	require.Equal(t, pcma(1000, mixerChunkSize), chunks[5])
	require.Equal(t, pcma(1000, mixerChunkSize/2), chunks[10][:mixerChunkSize/2])
	require.Equal(t, silence, chunks[12])

	require.Equal(t, 6, stats.Chunks)
	require.Equal(t, 0, stats.Mixed)
	require.Equal(t, 0, stats.Underruns)
}

func TestMixerStartDelay(t *testing.T) {
	var mu sync.Mutex
	var chunks int
	var setup time.Time

	m := newMixer(mixerOptions{}, func(chunk []byte) error {
		mu.Lock()
		chunks++
		mu.Unlock()
		return nil
	}, nil)

	begin := time.Now()
	m.start(200*time.Millisecond, func() error {
		setup = time.Now()
		return nil
	})

	// the audio is queued during the delay and played after a Stop
	src := m.AddSource()
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize*3)})
	m.Stop(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, setup.Sub(begin), 200*time.Millisecond)
	require.Equal(t, 3, chunks)

	// nothing queued: Stop doesn't wait for the delay
	m = newMixer(mixerOptions{}, func([]byte) error { return nil }, nil)
	called := false
	m.start(10*time.Second, func() error {
		called = true
		return nil
	})
	begin = time.Now()
	m.Stop(5 * time.Second)
	require.Less(t, time.Since(begin), time.Second)
	require.False(t, called)
}

func TestAddTrackAfterFailedSession(t *testing.T) {
	// a mixer that has finished after a write error
	m := newMixer(mixerOptions{}, nil, nil)
	m.start(0, func() error { return errors.New("broken pipe") })
	require.Eventually(t, m.Finished, time.Second, 10*time.Millisecond)

	conn2, peer := net.Pipe()
	defer peer.Close()

	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}
	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
	old := core.NewSender(media, codec)

	// nothing listens on the port, so the new talk session fails at once
	u, _ := url.Parse("tapo://127.0.0.1:1?backchannel_pace=0")
	c := &Client{
		url:         u,
		session1:    "1",
		conn2:       conn2,
		session2:    "2",
		mixer:       m,
		backSenders: []backchannelSender{{sender: old, source: m.AddSource()}},
	}

	err := c.AddTrack(media, codec, core.NewReceiver(media, codec))
	require.Error(t, err)

	// the old session is closed, the next source tries a new one
	require.Nil(t, c.mixer)
	require.Empty(t, c.backSenders)
	require.Nil(t, c.conn2)
	require.Empty(t, c.session2)
	require.Equal(t, "closed", old.State())
	_, err = conn2.Write([]byte{0})
	require.ErrorIs(t, err, io.ErrClosedPipe)
}
