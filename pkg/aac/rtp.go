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

		if len(packet.Payload) < int(2+headersSize) {
			return
		}

		headers := packet.Payload[2 : 2+headersSize]
		units := packet.Payload[2+headersSize:]

		for len(headers) > 0 {
			unitSize := binary.BigEndian.Uint16(headers) >> 3

			unit := units[:unitSize]

			headers = headers[2:]
			units = units[unitSize:]

			timestamp += AUTime

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

func ADTStoRTP(src []byte) (dst []byte) {
	dst = make([]byte, 2) // header bytes
	for i, n := 0, len(src)-ADTSHeaderSize; i < n; {
		auSize := ReadADTSSize(src[i:])
		dst = append(dst, byte(auSize>>5), byte(auSize<<3)) // size in bits
		i += int(auSize)
	}
	hdrSize := uint16(len(dst) - 2)
	binary.BigEndian.PutUint16(dst, hdrSize<<3) // size in bits
	return append(dst, src...)
}

func RTPTimeSize(b []byte) uint32 {
	// convert RTP header size to units count
	units := binary.BigEndian.Uint16(b) >> 4
	return uint32(units) * AUTime
}

func RTPToADTS(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	adts := CodecToADTS(codec)

	return func(packet *rtp.Packet) {
		src := packet.Payload
		dst := make([]byte, 0, len(src))

		headersSize := binary.BigEndian.Uint16(src) >> 3
		headers := src[2 : 2+headersSize]
		units := src[2+headersSize:]

		for len(headers) > 0 {
			unitSize := binary.BigEndian.Uint16(headers) >> 3
			headers = headers[2:]
			unit := units[:unitSize]
			units = units[unitSize:]

			if !IsADTS(unit) {
				i := len(dst)
				dst = append(dst, adts...)
				WriteADTSSize(dst[i:], ADTSHeaderSize+uint16(len(unit)))
			}

			dst = append(dst, unit...)
		}

		clone := *packet
		clone.Version = RTPPacketVersionAAC
		clone.Payload = dst
		handler(&clone)
	}
}

func RTPToCodec(b []byte) *core.Codec {
	hdrSize := binary.BigEndian.Uint16(b) / 8
	return ADTSToCodec(b[2+hdrSize:])
}
