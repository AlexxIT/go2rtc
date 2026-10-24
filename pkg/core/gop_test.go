package core

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// keyframe packets are marked by payload[0] == 'K' in these tests.
func isK(p *Packet) bool { return len(p.Payload) > 0 && p.Payload[0] == 'K' }

func pkt(kind byte, ts uint32, seq uint16) *Packet {
	p := &Packet{Payload: []byte{kind}}
	p.Timestamp = ts
	p.SequenceNumber = seq
	return p
}

func collect(t *testing.T, r *Receiver, fromKeyframe bool) (*Sender, func() []uint16) {
	var mu sync.Mutex
	var got []uint16
	s := NewSender(nil, &Codec{Name: CodecH264})
	s.Handler = func(p *Packet) {
		mu.Lock()
		got = append(got, p.SequenceNumber)
		mu.Unlock()
	}
	if fromKeyframe {
		s.WithParentFromKeyframe(r)
	} else {
		s.WithParent(r)
	}
	s.Start()
	return s, func() []uint16 {
		s.Close()
		s.Wait()
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

func TestGOPReplayFromKeyframe(t *testing.T) {
	r := NewReceiver(nil, &Codec{Name: CodecH264})
	r.KeepGOP(isK)

	r.Input(pkt('P', 1, 1)) // before any keyframe: not cached
	r.Input(pkt('K', 2, 2)) // SPS
	r.Input(pkt('K', 2, 3)) // IDR of the same access unit: same GOP
	r.Input(pkt('P', 3, 4))

	_, late := collect(t, r, true)
	_, plain := collect(t, r, false)
	r.Input(pkt('P', 4, 5))

	require.Equal(t, []uint16{2, 3, 4, 5}, late())
	require.Equal(t, []uint16{5}, plain())
}

func TestGOPRestartsAtNextKeyframe(t *testing.T) {
	r := NewReceiver(nil, &Codec{Name: CodecH264})
	r.KeepGOP(isK)
	r.Input(pkt('K', 1, 1))
	r.Input(pkt('P', 2, 2))
	r.Input(pkt('K', 3, 3))
	r.Input(pkt('P', 4, 4))

	_, late := collect(t, r, true)
	require.Equal(t, []uint16{3, 4}, late())
}

func TestGOPBounded(t *testing.T) {
	r := NewReceiver(nil, &Codec{Name: CodecH264})
	r.KeepGOP(isK)
	r.Input(pkt('K', 1, 0))
	for i := 1; i <= gopMaxPackets; i++ {
		r.Input(pkt('P', uint32(i+1), uint16(i)))
	}
	require.Empty(t, r.gop.Load().packets)
}

func TestGOPSurvivesReplace(t *testing.T) {
	r := NewReceiver(nil, &Codec{Name: CodecH264})
	r.KeepGOP(isK)
	target := NewReceiver(nil, &Codec{Name: CodecH264})
	r.Replace(target)
	require.NotNil(t, target.gop.Load())
}

// A consumer may ask for the cache while packets are flowing.
func TestGOPEnabledWhileStreaming(t *testing.T) {
	r := NewReceiver(nil, &Codec{Name: CodecH264})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			r.Input(pkt('P', uint32(i), uint16(i)))
		}
		close(done)
	}()
	r.KeepGOP(isK)
	_, late := collect(t, r, true)
	<-done
	_ = late()
}
