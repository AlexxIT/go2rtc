package av1

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAV1 = 0

// maxUnitBytes bounds both the reassembled temporal unit and the fragments the
// depacketizer is still holding, so a stream that never completes one cannot
// grow either without end.
const maxUnitBytes = 5 * 1024 * 1024

// RTPDepay converts AV1 RTP packets (RFC 9583) into AV1 temporal units
// in low overhead bitstream format (OBUs with obu_has_size_field=1),
// suitable for MP4/fMP4 muxing.
//
// Uses pion's AV1Depacketizer for correct handling of OBU fragmentation,
// aggregation headers, and size field conversion.
func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	depack := &codecs.AV1Depacketizer{}
	buf := make([]byte, 0, 512*1024)
	var seqNum uint16
	var fragBytes int
	var hasSeq, resync, started bool

	reset := func() {
		if cap(buf) > 512*1024 {
			buf = make([]byte, 0, 512*1024)
		} else {
			buf = buf[:0]
		}
		depack = &codecs.AV1Depacketizer{}
		fragBytes = 0
		started = false
	}

	return func(packet *rtp.Packet) {
		// A gap splices OBUs from different units together, and pion's
		// depacketizer rewrites obu_size to match, so the result still parses
		// as a valid keyframe. Drop the unit instead.
		gap := hasSeq && packet.SequenceNumber-seqNum != 1
		seqNum = packet.SequenceNumber
		hasSeq = true

		if gap {
			reset()
			resync = true
		}

		// Resuming at the next packet would collect the tail of the broken unit
		// and splice it onto the following one, so wait for a marker and start
		// with the unit after it.
		if resync {
			if packet.Marker {
				reset()
				resync = false
			}
			return
		}

		// Z=1 means the first OBU continues one from the previous packet. Before
		// we accepted anything it belongs to a unit we never saw, which happens
		// when a stream is joined mid unit.
		if !started && len(packet.Payload) > 0 && packet.Payload[0]&0x80 != 0 {
			reset()
			resync = true
			return
		}

		// The depacketizer holds fragments of an unfinished OBU itself and
		// returns nothing meanwhile, so buf below never sees them. Without a
		// bound here a run of continuation fragments grows its buffer without
		// end, and it recopies that buffer per packet.
		if fragBytes+len(packet.Payload) > maxUnitBytes {
			reset()
			resync = true
			return
		}

		payload, err := depack.Unmarshal(packet.Payload)
		if err != nil {
			// half a temporal unit would be left over otherwise
			reset()
			resync = true
			return
		}
		fragBytes += len(packet.Payload)
		started = true

		if len(payload) == 0 {
			return
		}

		// can happen if we miss a lot of packets with the marker
		if len(buf)+len(payload) > maxUnitBytes {
			reset()
			resync = true
			return
		}

		// Collect OBUs for the complete temporal unit
		buf = append(buf, payload...)

		if !packet.Marker {
			return // wait for complete temporal unit
		}

		// Make a copy to avoid aliasing - buf is reused across calls
		payload2 := make([]byte, len(buf))
		copy(payload2, buf)
		buf = buf[:0]
		fragBytes = 0
		started = false

		clone := *packet
		clone.Version = RTPPacketVersionAV1
		clone.Payload = payload2
		clone.ExtensionProfile = 0 // AV1 has no B-frames, CTS always equals DTS

		handler(&clone)
	}
}

// RTPPay packetizes AV1 OBUs into RTP packets with proper MTU fragmentation.
// Input packets must have Version == RTPPacketVersionAV1 with reassembled OBUs.
func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &codecs.AV1Payloader{}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != RTPPacketVersionAV1 {
			handler(packet)
			return
		}

		payloads := payloader.Payload(mtu, packet.Payload)
		last := len(payloads) - 1
		for i, payload := range payloads {
			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         i == last,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: payload,
			}
			handler(&clone)
		}
	}
}
