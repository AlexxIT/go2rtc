package av1

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

// RTPDepay - depacketize AV1 RTP packets (https://aomediacodec.github.io/av1-rtp-spec/)
// into temporal units in the AV1 low overhead bitstream format (OBUs with obu_size_field).
// One output packet = one temporal unit (same convention as h264/h265 access units).
func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	depack := &codecs.AV1Depacketizer{}

	buf := make([]byte, 0, 512*1024) // 512K
	var seqNum uint16

	return func(packet *rtp.Packet) {
		// when we collect data into one buffer, we need to make sure
		// that all of it falls into the same sequence
		if len(buf) > 0 && packet.SequenceNumber-seqNum != 1 {
			//log.Printf("broken AV1 sequence")
			buf = buf[:0]                        // drop data
			depack = &codecs.AV1Depacketizer{}   // drop pending OBU fragment
			return
		}

		seqNum = packet.SequenceNumber

		obus, err := depack.Unmarshal(packet.Payload)
		if err != nil {
			buf = buf[:0]
			depack = &codecs.AV1Depacketizer{}
			return
		}

		buf = append(buf, obus...)

		// collect all OBUs for temporal unit
		if !packet.Marker {
			return
		}

		if len(buf) == 0 {
			return
		}

		clone := *packet
		clone.Version = h264.RTPPacketVersionAVC
		clone.Payload = buf

		buf = buf[:0]

		handler(&clone)
	}
}

// RTPPay - packetize AV1 temporal units (low overhead bitstream) into RTP packets
// sized for the given MTU, per the AV1 RTP specification.
func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &codecs.AV1Payloader{}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != h264.RTPPacketVersionAVC {
			handler(packet)
			return
		}

		payloads := payloader.Payload(mtu, packet.Payload)
		last := len(payloads) - 1
		for i, payload := range payloads {
			// 4K AV1 keyframe temporal units run to hundreds of packets;
			// blasting them in one tight loop overflows UDP socket buffers
			// (~50% observed loss on a gigabit LAN). Pace large bursts —
			// each sender runs on its own goroutine, so sleeping here only
			// delays this consumer, never the producer.
			if i > 0 && i%16 == 0 {
				time.Sleep(2 * time.Millisecond)
			}
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
