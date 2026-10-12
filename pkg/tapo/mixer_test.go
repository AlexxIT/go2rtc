package tapo

import (
	"bytes"
	"sync"
	"testing"
	"time"

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
	for n := 2; n <= 4; n++ {
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
	src = m.AddSource()
	src.Push(&rtp.Packet{Payload: pcma(1000, mixerChunkSize)})
	require.Len(t, src.queue, mixerChunkSize)
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
	go m.run()

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
