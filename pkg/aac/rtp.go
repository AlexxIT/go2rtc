package aac

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

const RTPPacketVersionAAC = 0

func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	var timestamp uint32

	return func(packet *rtp.Packet) {
		// support ONLY 2 bytes header size!
		// streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1408
		// https://datatracker.ietf.org/doc/html/rfc3640
		headersSize := rtpAUHeadersSize(packet.Payload)
		// Reject malformed AU-header lengths and sections truncated by the payload.
		if headersSize < 0 || len(packet.Payload) < 2+headersSize {
			// In very rare cases noname cameras may send data not according to the standard
			// Compatibility workaround for cameras that send ADTS directly.
			if IsADTS(packet.Payload) {
				clone := *packet
				clone.Version = RTPPacketVersionAAC
				clone.Timestamp = timestamp
				clone.Payload = clone.Payload[ADTSHeaderSize:]
				handler(&clone)
			}
			return
		}

		//log.Printf("[RTP/AAC] units: %d, size: %4d, ts: %10d, %t", headersSize/2, len(packet.Payload), packet.Timestamp, packet.Marker)

		headers := packet.Payload[2 : 2+headersSize]
		units := packet.Payload[2+headersSize:]

		for len(headers) >= 2 {
			unitSize := binary.BigEndian.Uint16(headers) >> 3

			if len(units) < int(unitSize) {
				return
			}

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
	var seq uint16
	var ts uint32

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
				SequenceNumber: seq,
				Timestamp:      ts,
			},
			Payload: payload,
		}
		handler(&clone)

		seq++
		ts += AUTime
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
	headerSize := rtpAUHeadersSize(b)
	if headerSize < 0 || len(b) < 2+headerSize {
		return 0
	}
	return uint32(headerSize/2) * AUTime
}

func RTPToADTS(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	adts := CodecToADTS(codec)

	return func(packet *rtp.Packet) {
		src := packet.Payload
		headersSize := rtpAUHeadersSize(src)
		if headersSize < 0 || len(src) < 2+headersSize {
			return
		}
		dst := make([]byte, 0, len(src))

		headers := src[2 : 2+headersSize]
		units := src[2+headersSize:]

		for len(headers) > 0 {
			unitSize := int(binary.BigEndian.Uint16(headers) >> 3)
			headers = headers[2:]
			if unitSize > len(units) {
				return
			}
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
	headerSize := rtpAUHeadersSize(b)
	if headerSize < 2 || len(b) < 2+headerSize {
		return nil
	}

	unitSize := int(binary.BigEndian.Uint16(b[2:4]) >> 3)
	units := b[2+headerSize:]
	if unitSize > len(units) {
		return nil
	}
	return ADTSToCodec(units[:unitSize])
}

// rtpAUHeadersSize validates the two-byte AU-headers-length field. AAC RTP
// uses 16-bit AU headers (13-bit size and 3-bit index).
func rtpAUHeadersSize(b []byte) int {
	if len(b) < 2 {
		return -1
	}
	bits := binary.BigEndian.Uint16(b)
	// AU headers are 16 bits each, so the section must contain a whole number
	// of them.
	if bits%16 != 0 {
		return -1
	}
	return int(bits / 8)
}
