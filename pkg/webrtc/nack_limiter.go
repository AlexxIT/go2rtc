package webrtc

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// Limits for answering NACKs (RFC 4585 generic NACK).
//
// pion's NACK responder resends every requested packet that is still buffered.
// Its only rate brake reads the target bitrate set by the GCC interceptor,
// which go2rtc does not register, so without a limit one peer can make go2rtc
// send ~1000x its own NACK traffic (a 256-byte NACK covers the 1024 buffered
// packets). The limiter below mirrors libwebrtc: retransmissions may use at
// most as many bytes as the stream itself sent in the last window.
const (
	// nackBufferSize is the responder's per-SSRC packet history. The limiter
	// keeps the same window so it knows which sequence numbers can be resent.
	nackBufferSize = 1024

	// nackWindow is the sliding window for both the send rate and the resend budget.
	nackWindow = 500 * time.Millisecond

	// nackMaxResendBytesPerSec caps retransmissions per stream even for
	// high-bitrate sources (~16.8 Mbit/s, above typical camera streams).
	nackMaxResendBytesPerSec = 2 << 20

	// nackMinResendBytesPerWindow lets a quiet or low-bitrate stream still repair
	// a short burst loss (~27 packets of 1200 bytes, ~0.5 Mbit/s).
	nackMinResendBytesPerWindow = 32 << 10

	// nackMaxResendsPerSeq bounds how often one packet is resent. Browsers give
	// up on a packet after a few retries; more only feeds a lossy relay path.
	nackMaxResendsPerSeq = 5
)

const nackBucketCount = 10

// windowCounter sums bytes over the last nackWindow in fixed buckets.
type windowCounter struct {
	bytes [nackBucketCount]int64
	stamp [nackBucketCount]int64
}

func (w *windowCounter) bucket(now time.Time) int64 {
	return now.UnixNano() / int64(nackWindow/nackBucketCount)
}

func (w *windowCounter) add(now time.Time, n int64) {
	idx := w.bucket(now)
	slot := idx % nackBucketCount
	if w.stamp[slot] != idx {
		w.stamp[slot] = idx
		w.bytes[slot] = 0
	}
	w.bytes[slot] += n
}

func (w *windowCounter) sum(now time.Time) (total int64) {
	idx := w.bucket(now)
	for i := range w.bytes {
		if idx-w.stamp[i] < nackBucketCount {
			total += w.bytes[i]
		}
	}
	return
}

type nackHistoryEntry struct {
	seq     uint16
	size    int64
	resends int
	valid   bool
}

type nackLimiterStream struct {
	history [nackBufferSize]nackHistoryEntry
	sent    windowCounter
	resent  windowCounter
}

// nackLimiterFactory creates a nackLimiter per PeerConnection.
type nackLimiterFactory struct {
	maxResendBytesPerSec int
	maxResendsPerSeq     int
}

// newNackLimiter returns an interceptor that removes NACK entries beyond the
// resend budget before pion's NACK responder sees them.
//
// It must be registered BEFORE the responder: interceptor.Chain wraps RTCP
// readers in registry order (pion/interceptor@v0.1.51 chain.go:19-25), so the
// first registered interceptor sits next to the transport, and the responder
// only acts after its inner reader returned (pkg/nack/responder_interceptor.go:103-117).
// Both share the parsed packets through Attributes.GetRTCPPackets (attributes.go:62-78).
func newNackLimiter(maxResendBytesPerSec, maxResendsPerSeq int) interceptor.Factory {
	return &nackLimiterFactory{maxResendBytesPerSec: maxResendBytesPerSec, maxResendsPerSeq: maxResendsPerSeq}
}

func (f *nackLimiterFactory) NewInterceptor(string) (interceptor.Interceptor, error) {
	return &nackLimiter{
		// int64: on 32-bit platforms bytes * nanoseconds overflows int.
		maxWindowBytes:   int64(f.maxResendBytesPerSec) * int64(nackWindow) / int64(time.Second),
		maxResendsPerSeq: f.maxResendsPerSeq,
		streams:          map[uint32]*nackLimiterStream{},
		now:              time.Now,
	}, nil
}

type nackLimiter struct {
	interceptor.NoOp

	maxWindowBytes   int64
	maxResendsPerSeq int
	now              func() time.Time

	mu      sync.Mutex
	streams map[uint32]*nackLimiterStream
}

// BindLocalStream records size and send time of every original packet. Being
// first in the chain it also sees the responder's retransmissions; those reuse
// a sequence number already in the history and are not counted as sent bytes.
func (l *nackLimiter) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	stream := &nackLimiterStream{}
	l.mu.Lock()
	l.streams[info.SSRC] = stream
	l.mu.Unlock()

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attr interceptor.Attributes) (int, error) {
		size := int64(header.MarshalSize() + len(payload))
		l.mu.Lock()
		entry := &stream.history[header.SequenceNumber%nackBufferSize]
		if !entry.valid || entry.seq != header.SequenceNumber {
			*entry = nackHistoryEntry{seq: header.SequenceNumber, size: size, valid: true}
			stream.sent.add(l.now(), size)
		}
		l.mu.Unlock()
		return writer.Write(header, payload, attr)
	})
}

func (l *nackLimiter) UnbindLocalStream(info *interceptor.StreamInfo) {
	l.mu.Lock()
	delete(l.streams, info.SSRC)
	l.mu.Unlock()
}

func (l *nackLimiter) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, attr, err := reader.Read(b, a)
		if err != nil {
			return n, attr, err
		}
		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		pkts, err := attr.GetRTCPPackets(b[:n])
		if err != nil {
			return 0, nil, err
		}

		changed := false
		for _, pkt := range pkts {
			if nack, ok := pkt.(*rtcp.TransportLayerNack); ok {
				if l.filter(nack) {
					changed = true
				}
			}
		}

		// The responder reads the (now filtered) cached packets; keep the raw
		// bytes consistent for any other consumer.
		if changed {
			if raw, err := marshalWithoutEmptyNacks(pkts); err == nil && len(raw) <= len(b) {
				n = copy(b, raw)
			}
		}
		return n, attr, nil
	})
}

// marshalWithoutEmptyNacks marshals pkts but skips NACKs the limiter emptied.
// A NACK without FCI is invalid (RFC 4585 6.2.1) and rtcp.Unmarshal rejects
// it together with the rest of the compound, e.g. a PLI next to it. The
// cached slice is left as is, so the responder still sees the emptied NACK.
// If every packet is skipped, rtcp.Marshal returns no bytes.
func marshalWithoutEmptyNacks(pkts []rtcp.Packet) ([]byte, error) {
	keep := make([]rtcp.Packet, 0, len(pkts))
	for _, pkt := range pkts {
		if nack, ok := pkt.(*rtcp.TransportLayerNack); ok && len(nack.Nacks) == 0 {
			continue
		}
		keep = append(keep, pkt)
	}
	return rtcp.Marshal(keep)
}

// filter rewrites nack.Nacks to the sequence numbers allowed by the budget.
// It reports whether anything was removed.
func (l *nackLimiter) filter(nack *rtcp.TransportLayerNack) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	stream := l.streams[nack.MediaSSRC]
	if stream == nil {
		if len(nack.Nacks) == 0 {
			return false
		}
		nack.Nacks = nil // nothing of ours to resend
		return true
	}

	budget := stream.sent.sum(now)
	if budget < nackMinResendBytesPerWindow {
		budget = nackMinResendBytesPerWindow
	}
	if budget > l.maxWindowBytes {
		budget = l.maxWindowBytes
	}
	used := stream.resent.sum(now)

	var allowed []uint16
	requested := 0
	for i := range nack.Nacks {
		nack.Nacks[i].Range(func(seq uint16) bool {
			requested++
			entry := &stream.history[seq%nackBufferSize]
			if !entry.valid || entry.seq != seq || entry.resends >= l.maxResendsPerSeq {
				return true
			}
			if used+entry.size > budget {
				return true
			}
			used += entry.size
			entry.resends++
			stream.resent.add(now, entry.size)
			allowed = append(allowed, seq)
			return true
		})
	}

	if len(allowed) == requested {
		return false
	}
	nack.Nacks = rtcp.NackPairsFromSequenceNumbers(allowed)
	return true
}
