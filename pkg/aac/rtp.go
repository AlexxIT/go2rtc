package aac

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

const RTPPacketVersionAAC = 0
const ADTSHeaderSize = 7

func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	var timestamp uint32

	return func(packet *rtp.Packet) {
		// support ONLY 2 bytes header size!
		// streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1408
		// https://datatracker.ietf.org/doc/html/rfc3640
		headersSize := binary.BigEndian.Uint16(packet.Payload) >> 3

		//log.Printf("[RTP/AAC] units: %d, size: %4d, ts: %10d, %t", headersSize/2, len(packet.Payload), packet.Timestamp, packet.Marker)

		headers := packet.Payload[2 : 2+headersSize]
		units := packet.Payload[2+headersSize:]

		for len(headers) > 0 {
			unitSize := binary.BigEndian.Uint16(headers) >> 3

			unit := units[:unitSize]

			headers = headers[2:]
			units = units[unitSize:]

			timestamp += 1024

			clone := *packet
			clone.Version = RTPPacketVersionAAC
			clone.Timestamp = timestamp
			if IsADTS(unit) {
				clone.Payload = unit[ADTSHeaderSize:]
			} else {
				clone.Payload = unit
			}
			handler(&clone)
		}
	}
}

func RTPPay(handler core.HandlerFunc) core.HandlerFunc {
	sequencer := rtp.NewRandomSequencer()

	return func(packet *rtp.Packet) {
		if packet.Version != RTPPacketVersionAAC {
			handler(packet)
			return
		}

		// support ONLY one unit in payload
		auSize := uint16(len(packet.Payload))
		// 2 bytes header size + 2 bytes first payload size
		payload := make([]byte, 2+2+auSize)
		payload[1] = 16 // header size in bits
		binary.BigEndian.PutUint16(payload[2:], auSize<<3)
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
		handler(&clone)
	}
}

func ADTStoRTP(b []byte) []byte {
	header := make([]byte, 2)
	for i := 0; i < len(b); {
		auSize := ReadADTSSize(b[i:])
		header = append(header, byte(auSize>>5), byte(auSize<<3)) // size in bits
		i += int(auSize)
	}
	hdrSize := uint16(len(header) - 2)
	binary.BigEndian.PutUint16(header, hdrSize<<3) // size in bits
	return append(header, b...)
}

func RTPToCodec(b []byte) *core.Codec {
	hdrSize := binary.BigEndian.Uint16(b) / 8
	return ADTSToCodec(b[2+hdrSize:])
}
