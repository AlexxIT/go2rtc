package baichuan

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	bcmediaInfoV1      = 0x31303031
	bcmediaInfoV2      = 0x32303031
	bcmediaIFrameMin   = 0x63643030
	bcmediaIFrameMax   = 0x63643039
	bcmediaPFrameMin   = 0x63643130
	bcmediaPFrameMax   = 0x63643139
	bcmediaAAC         = 0x62773530
	bcmediaAACV2       = 0x62773531
	bcmediaADPCM       = 0x62773130
	bcmediaADPCMHeader = 0x0100
	bcmediaPadSize     = 8
)

// MediaParser incrementally parses the bcmedia byte stream carried by msg_id=3.
type MediaParser struct {
	buf bytes.Buffer
}

// Append adds bytes to the parser and returns every complete media packet found.
func (p *MediaParser) Append(data []byte) ([]MediaPacket, error) {
	if len(data) > 0 {
		_, _ = p.buf.Write(data)
	}

	var out []MediaPacket
	for {
		packet, consumed, ok, err := parseMediaPacket(p.buf.Bytes())
		if err != nil {
			return out, err
		}
		if !ok {
			return out, nil
		}

		p.buf.Next(consumed)
		out = append(out, packet)
	}
}

func parseMediaPacket(buf []byte) (MediaPacket, int, bool, error) {
	if len(buf) < 4 {
		return MediaPacket{}, 0, false, nil
	}

	magic := binary.LittleEndian.Uint32(buf[0:4])
	switch {
	case magic == bcmediaInfoV1 || magic == bcmediaInfoV2:
		if len(buf) < 32 {
			return MediaPacket{}, 0, false, nil
		}
		headerSize := binary.LittleEndian.Uint32(buf[4:8])
		if headerSize != 32 {
			return MediaPacket{}, 0, false, fmt.Errorf("unexpected bcmedia info header size %d", headerSize)
		}

		packet := MediaPacket{
			Kind:   MediaPacketInfoV1,
			Width:  binary.LittleEndian.Uint32(buf[8:12]),
			Height: binary.LittleEndian.Uint32(buf[12:16]),
			FPS:    buf[17],
		}
		if magic == bcmediaInfoV2 {
			packet.Kind = MediaPacketInfoV2
		}
		return packet, 32, true, nil

	case magic >= bcmediaIFrameMin && magic <= bcmediaIFrameMax:
		return parseVideoFrame(buf, true)

	case magic >= bcmediaPFrameMin && magic <= bcmediaPFrameMax:
		return parseVideoFrame(buf, false)

	case magic == bcmediaAAC || magic == bcmediaAACV2:
		if len(buf) < 8 {
			return MediaPacket{}, 0, false, nil
		}
		payloadSize := int(binary.LittleEndian.Uint16(buf[4:6]))
		total := 8 + payloadSize + padLen(payloadSize)
		if len(buf) < total {
			return MediaPacket{}, 0, false, nil
		}

		packet := MediaPacket{
			Kind: MediaPacketAAC,
			Data: append([]byte(nil), buf[8:8+payloadSize]...),
		}
		return packet, total, true, nil

	case magic == bcmediaADPCM:
		if len(buf) < 12 {
			return MediaPacket{}, 0, false, nil
		}
		payloadSize := int(binary.LittleEndian.Uint16(buf[4:6]))
		total := 8 + payloadSize + padLen(payloadSize)
		if len(buf) < total {
			return MediaPacket{}, 0, false, nil
		}
		if binary.LittleEndian.Uint16(buf[8:10]) != bcmediaADPCMHeader {
			return MediaPacket{}, 0, false, fmt.Errorf("unexpected adpcm marker %#x", binary.LittleEndian.Uint16(buf[8:10]))
		}

		blockSize := payloadSize - 4
		packet := MediaPacket{
			Kind: MediaPacketADPCM,
			Data: append([]byte(nil), buf[12:12+blockSize]...),
		}
		return packet, total, true, nil

	default:
		return MediaPacket{}, 0, false, fmt.Errorf("unknown bcmedia magic %#x", magic)
	}
}

func parseVideoFrame(buf []byte, iframe bool) (MediaPacket, int, bool, error) {
	if len(buf) < 24 {
		return MediaPacket{}, 0, false, nil
	}

	codec := string(buf[4:8])
	if codec != "H264" && codec != "H265" {
		return MediaPacket{}, 0, false, fmt.Errorf("unsupported video codec %q", codec)
	}

	payloadSize := int(binary.LittleEndian.Uint32(buf[8:12]))
	additionalHeaderSize := int(binary.LittleEndian.Uint32(buf[12:16]))
	microseconds := binary.LittleEndian.Uint32(buf[16:20])

	total := 24 + additionalHeaderSize + payloadSize + padLen(payloadSize)
	if len(buf) < total {
		return MediaPacket{}, 0, false, nil
	}

	pos := 24
	var unixTime *time.Time
	if iframe && additionalHeaderSize >= 4 {
		ts := int64(binary.LittleEndian.Uint32(buf[pos : pos+4]))
		t := time.Unix(ts, 0).UTC()
		unixTime = &t
	}
	pos += additionalHeaderSize

	packet := MediaPacket{
		Kind:               MediaPacketPFrame,
		Codec:              codec,
		Data:               append([]byte(nil), buf[pos:pos+payloadSize]...),
		TimestampMicrosecs: microseconds,
		HasTimestamp:       true,
		UnixTime:           unixTime,
	}
	if iframe {
		packet.Kind = MediaPacketIFrame
	}

	return packet, total, true, nil
}

func padLen(size int) int {
	if size%bcmediaPadSize == 0 {
		return 0
	}
	return bcmediaPadSize - (size % bcmediaPadSize)
}
