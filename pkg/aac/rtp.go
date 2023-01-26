package aac

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

const RTPPacketVersionAAC = 0

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			// support ONLY 2 bytes header size!
			// streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1408
			headersSize := binary.BigEndian.Uint16(packet.Payload) >> 3

			//log.Printf("[RTP/AAC] units: %d, size: %4d, ts: %10d, %t", headersSize/2, len(packet.Payload), packet.Timestamp, packet.Marker)

			data := packet.Payload[2+headersSize:]
			if IsADTS(data) {
				data = data[7:]
			}

			clone := *packet
			clone.Version = RTPPacketVersionAAC
			clone.Payload = data
			return push(&clone)
		}
	}
}

func RTPPay(mtu uint16) streamer.WrapperFunc {
	sequencer := rtp.NewRandomSequencer()

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version != RTPPacketVersionAAC {
				return push(packet)
			}

			// support ONLY one unit in payload
			size := uint16(len(packet.Payload))
			// 2 bytes header size + 2 bytes first payload size
			payload := make([]byte, 2+2+size)
			payload[1] = 16 // header size in bits
			binary.BigEndian.PutUint16(payload[2:], size<<3)
			copy(payload[4:], packet.Payload)

			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: payload,
			}
			return push(&clone)
		}
	}
}

func IsADTS(b []byte) bool {
	return len(b) > 7 && b[0] == 0xFF && b[1]&0xF0 == 0xF0
}
