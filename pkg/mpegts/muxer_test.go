package mpegts

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMuxerHeader(t *testing.T) {
	m := NewMuxer()
	require.Equal(t, uint16(pes0PID), m.AddTrack(StreamTypeH264, ClockRate))
	require.Equal(t, uint16(pes0PID+1), m.AddTrack(StreamTypeAAC, 48000))

	d := NewDemuxer()
	_, err := d.ReadPacket(bytes.NewReader(m.GetHeader()))
	require.NoError(t, err)
	require.Len(t, d.pes, 2)
	require.Equal(t, byte(StreamTypeH264), d.pes[pes0PID].StreamType)
	require.Equal(t, byte(StreamTypeAAC), d.pes[pes0PID+1].StreamType)
}

// A track can be added after the sender of another track has started,
// e.g. when the audio source of a stream appears later than the video.
func TestMuxerAddTrackWhileWriting(t *testing.T) {
	for range 100 {
		m := NewMuxer()
		pid := m.AddTrack(StreamTypeH264, ClockRate)

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ts := uint32(0); ; ts += 3000 {
				select {
				case <-stop:
					return
				default:
					_ = m.GetPayload(pid, ts, []byte{0, 0, 0, 1, 0x65})
				}
			}
		}()

		m.AddTrack(StreamTypeAAC, 48000)
		_ = m.GetHeader()

		close(stop)
		wg.Wait()
	}
}

func TestMuxerTimestampConversion(t *testing.T) {
	m := NewMuxer()
	pid := m.AddTrack(StreamTypeAAC, 8000)
	for i := range uint32(10) {
		m.GetPayload(pid, i*320, []byte{0xFF})
	}
	require.Equal(t, uint32(9*3600), m.pes[pid].PTS)
}

func TestMuxerTimestampWraparound(t *testing.T) {
	m := NewMuxer()
	pid := m.AddTrack(StreamTypeAAC, 8000)
	m.GetPayload(pid, 0xFFFFFFF0, []byte{0xFF})
	m.GetPayload(pid, 0x10, []byte{0xFF})
	require.Equal(t, uint32(360), m.pes[pid].PTS)
}

func TestMuxerZeroTimestamp(t *testing.T) {
	m := NewMuxer()
	pid := m.AddTrack(StreamTypeAAC, 8000)
	m.GetPayload(pid, 0, []byte{0xFF})
	m.GetPayload(pid, 320, []byte{0xFF})
	require.Equal(t, uint32(3600), m.pes[pid].PTS)
}

func TestMuxerTimestampRoundingDrift(t *testing.T) {
	m := NewMuxer()
	pid := m.AddTrack(StreamTypeAAC, 44100)
	ts := uint32(0xFFFFF000)
	m.GetPayload(pid, ts, []byte{0xFF})
	for range 44100 {
		ts++
		m.GetPayload(pid, ts, []byte{0xFF})
	}
	require.Equal(t, uint32(90000), m.pes[pid].PTS)
}
