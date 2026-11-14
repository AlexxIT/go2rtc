package webrtc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/require"
)

func TestWantsKeyframe(t *testing.T) {
	pli := &rtcp.PictureLossIndication{MediaSSRC: 1}
	fir := &rtcp.FullIntraRequest{MediaSSRC: 1, FIR: []rtcp.FIREntry{{SSRC: 1, SequenceNumber: 1}}}
	nack := &rtcp.TransportLayerNack{MediaSSRC: 1, Nacks: []rtcp.NackPair{{PacketID: 1}}}
	rr := &rtcp.ReceiverReport{SSRC: 1}

	tests := []struct {
		name string
		pkts []rtcp.Packet
		want bool
	}{
		{"pli", []rtcp.Packet{pli}, true},
		{"fir", []rtcp.Packet{fir}, true},
		{"rr+pli", []rtcp.Packet{rr, pli}, true},
		{"nack only", []rtcp.Packet{nack}, false},
		{"rr only", []rtcp.Packet{rr}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, wantsKeyframe(tt.pkts))
		})
	}
}

func TestRTCPReadDone(t *testing.T) {
	_, parseErr := rtcp.Unmarshal([]byte{0x00})
	require.Error(t, parseErr)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"eof", io.EOF, true},
		{"closed pipe", io.ErrClosedPipe, true},
		{"wrapped net closed", fmt.Errorf("srtcp: %w", net.ErrClosed), true},
		{"parse error", parseErr, false},
		{"other", errors.New("x"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, rtcpReadDone(tt.err))
		})
	}
}

// fakeRTCPReader replays a script of reads; once the script is exhausted it
// keeps returning tail (or io.ErrClosedPipe when tail is nil).
type fakeRTCPReader struct {
	steps []func(b []byte) (int, interceptor.Attributes, error)
	tail  error
	calls atomic.Int32
}

func (f *fakeRTCPReader) read(b []byte) (int, interceptor.Attributes, error) {
	i := int(f.calls.Add(1)) - 1
	if i < len(f.steps) {
		return f.steps[i](b)
	}
	if f.tail != nil {
		return 0, nil, f.tail
	}
	return 0, nil, io.ErrClosedPipe
}

func runReadRTCP(t *testing.T, r *fakeRTCPReader, onKeyframe func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		readRTCP(r.read, onKeyframe)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readRTCP did not return")
	}
}

func readErr(err error) func([]byte) (int, interceptor.Attributes, error) {
	return func([]byte) (int, interceptor.Attributes, error) { return 0, nil, err }
}

func readRaw(t *testing.T, pkts ...rtcp.Packet) func([]byte) (int, interceptor.Attributes, error) {
	raw, err := rtcp.Marshal(pkts)
	require.NoError(t, err)
	return func(b []byte) (int, interceptor.Attributes, error) { return copy(b, raw), nil, nil }
}

// A single unparsable packet (the limiter returns the parse error as a read
// error) must not end the loop; a PLI in the next read is still reported.
func TestReadRTCPContinuesAfterError(t *testing.T) {
	_, parseErr := rtcp.Unmarshal([]byte{0x00})
	r := &fakeRTCPReader{steps: []func([]byte) (int, interceptor.Attributes, error){
		readErr(parseErr),
		readRaw(t, &rtcp.PictureLossIndication{MediaSSRC: 1}),
		readErr(io.ErrClosedPipe),
	}, tail: errors.New("read after close")}

	var keyframes atomic.Int32
	runReadRTCP(t, r, func() { keyframes.Add(1) })
	require.EqualValues(t, 1, keyframes.Load())
	require.EqualValues(t, 3, r.calls.Load(), "loop must stop at io.ErrClosedPipe")
}

// Only errors without progress end the loop, after exactly maxRTCPReadErrors
// consecutive reads; a successful read in between resets the count.
func TestReadRTCPStopsAfterConsecutiveErrors(t *testing.T) {
	parse := errors.New("rtcp: invalid packet length")

	t.Run("errors only", func(t *testing.T) {
		r := &fakeRTCPReader{tail: parse}
		runReadRTCP(t, r, func() {})
		require.EqualValues(t, maxRTCPReadErrors, r.calls.Load())
	})

	t.Run("success resets", func(t *testing.T) {
		var steps []func([]byte) (int, interceptor.Attributes, error)
		for i := 0; i < maxRTCPReadErrors-1; i++ {
			steps = append(steps, readErr(parse))
		}
		steps = append(steps, readRaw(t, &rtcp.ReceiverReport{SSRC: 1}))
		r := &fakeRTCPReader{steps: steps, tail: parse}
		runReadRTCP(t, r, func() {})
		require.EqualValues(t, 2*maxRTCPReadErrors, r.calls.Load())
	})
}

// videoSender returns MID and SSRC of the go2rtc video sender.
func (p *loopbackPeers) videoSender(t *testing.T) (string, uint32) {
	t.Helper()
	for _, tr := range p.pc.GetTransceivers() {
		if s := tr.Sender(); s != nil && s.Track() == p.track {
			return tr.Mid(), uint32(s.GetParameters().Encodings[0].SSRC)
		}
	}
	t.Fatal("video sender not found")
	return "", 0
}

// countingHandler returns a handler and a channel that receives one value per call.
func countingHandler() (func(), chan struct{}) {
	calls := make(chan struct{}, 16)
	return func() { calls <- struct{}{} }, calls
}

func requireCalledOnce(t *testing.T, calls chan struct{}) {
	t.Helper()
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("keyframe handler not called")
	}
	select {
	case <-calls:
		t.Fatal("keyframe handler called more than once")
	case <-time.After(300 * time.Millisecond):
	}
}

func requireNotCalled(t *testing.T, calls chan struct{}) {
	t.Helper()
	select {
	case <-calls:
		t.Fatal("keyframe handler called unexpectedly")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestConnInvokesKeyframeHandlerOnPLI(t *testing.T) {
	p := newLoopbackPeers(t)
	mid, ssrc := p.videoSender(t)
	handler, calls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid}, handler)

	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: ssrc}}))
	requireCalledOnce(t, calls)
}

func TestConnInvokesKeyframeHandlerOnFIR(t *testing.T) {
	p := newLoopbackPeers(t)
	mid, ssrc := p.videoSender(t)
	handler, calls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid}, handler)

	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{&rtcp.FullIntraRequest{
		SenderSSRC: 1, MediaSSRC: ssrc, FIR: []rtcp.FIREntry{{SSRC: ssrc, SequenceNumber: 1}},
	}}))
	requireCalledOnce(t, calls)
}

// A compound [NACK for a foreign SSRC, PLI] passes the NACK limiter, which
// empties the NACK. The PLI must still be reported. Driven through the real
// limiter and readRTCP without a loopback: pion/srtp v3.1.3 splits a compound
// per destination SSRC (session_srtcp.go decrypt), so over a real connection a
// sender's read never carries the foreign NACK (see the loopback test below).
func TestReadRTCPKeyframeAfterFilteredNack(t *testing.T) {
	compound, err := rtcp.Marshal([]rtcp.Packet{
		&rtcp.TransportLayerNack{SenderSSRC: 1, MediaSSRC: 0xdeadbeef, Nacks: []rtcp.NackPair{{PacketID: 1}}},
		&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: 0x1234},
	})
	require.NoError(t, err)

	f := newNackLimiter(nackMaxResendBytesPerSec, nackMaxResendsPerSeq)
	i, err := f.NewInterceptor("")
	require.NoError(t, err)
	var reads atomic.Int32
	reader := i.BindRTCPReader(interceptor.RTCPReaderFunc(
		func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
			if reads.Add(1) > 1 {
				return 0, nil, io.ErrClosedPipe
			}
			return copy(b, compound), a, nil
		},
	))

	var keyframes atomic.Int32
	done := make(chan struct{})
	go func() {
		readRTCP(func(b []byte) (int, interceptor.Attributes, error) { return reader.Read(b, nil) },
			func() { keyframes.Add(1) })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readRTCP did not return")
	}
	require.EqualValues(t, 1, keyframes.Load())
}

// End-to-end over loopback: a compound with a NACK for a foreign SSRC and a
// PLI is sent; the PLI must reach the handler. Over a real connection srtp
// delivers the two packets separately, so this does not exercise the limiter
// emptying a NACK next to a PLI; TestReadRTCPKeyframeAfterFilteredNack does.
func TestConnKeyframeLoopbackCompoundWithForeignNack(t *testing.T) {
	p := newLoopbackPeers(t)
	mid, ssrc := p.videoSender(t)
	handler, calls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid}, handler)

	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{
		&rtcp.TransportLayerNack{SenderSSRC: 1, MediaSSRC: 0xdeadbeef, Nacks: []rtcp.NackPair{{PacketID: 1}}},
		&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: ssrc},
	}))
	requireCalledOnce(t, calls)
}

// A NACK alone is no keyframe request. The PLI afterwards proves the loop and
// the handler were live during the silent phase.
func TestConnNackDoesNotInvokeKeyframeHandler(t *testing.T) {
	p := newLoopbackPeers(t)
	mid, ssrc := p.videoSender(t)
	handler, calls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid}, handler)

	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{&rtcp.TransportLayerNack{
		SenderSSRC: 1, MediaSSRC: ssrc, Nacks: []rtcp.NackPair{{PacketID: 1}},
	}}))
	requireNotCalled(t, calls)

	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: ssrc}}))
	requireCalledOnce(t, calls)
}

// A handler registered for another MID must not see a PLI on the video
// sender. Registering the right MID afterwards proves the PLI path works.
func TestConnKeyframeHandlerOtherMid(t *testing.T) {
	p := newLoopbackPeers(t)
	mid, ssrc := p.videoSender(t)
	other, otherCalls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid + "-other"}, other)

	pli := []rtcp.Packet{&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: ssrc}}
	require.NoError(t, p.remote.WriteRTCP(pli))
	requireNotCalled(t, otherCalls)

	handler, calls := countingHandler()
	p.conn.OnKeyframeRequest(&core.Media{ID: mid}, handler)
	require.NoError(t, p.remote.WriteRTCP(pli))
	requireCalledOnce(t, calls)
	requireNotCalled(t, otherCalls)
}
