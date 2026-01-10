package tutk

import "encoding/binary"

const (
	// Start packets - first fragment of a frame
	// 0x08: Extended start (36-byte header, no FrameInfo)
	// 0x09: StartAlt (36-byte header, FrameInfo only if pkt_total==1)
	FrameTypeStart    uint8 = 0x08
	FrameTypeStartAlt uint8 = 0x09

	// Continuation packets - middle fragment (28-byte header, no FrameInfo)
	FrameTypeCont    uint8 = 0x00
	FrameTypeContAlt uint8 = 0x04

	// End packets - last fragment (with 40-byte FrameInfo)
	// 0x01: Single-packet frame (28-byte header)
	// 0x05: Multi-packet end (28-byte header)
	// 0x0d: Extended end (36-byte header)
	FrameTypeEndSingle uint8 = 0x01
	FrameTypeEndMulti  uint8 = 0x05
	FrameTypeEndExt    uint8 = 0x0d
)

const (
	ChannelIVideo uint8 = 0x05
	ChannelAudio  uint8 = 0x03
	ChannelPVideo uint8 = 0x07
)

type Packet struct {
	Channel    uint8
	Codec      uint16
	Timestamp  uint32
	Payload    []byte
	IsKeyframe bool
	FrameNo    uint32
	SampleRate uint32
	Channels   uint8
}

func (p *Packet) IsVideo() bool {
	return p.Channel == ChannelIVideo || p.Channel == ChannelPVideo
}

func (p *Packet) IsAudio() bool {
	return p.Channel == ChannelAudio
}

type AuthResponse struct {
	ConnectionRes string         `json:"connectionRes"`
	CameraInfo    map[string]any `json:"cameraInfo"`
}

type AVLoginResponse struct {
	ServerType      uint32
	Resend          int32
	TwoWayStreaming int32
	SyncRecvData    int32
	SecurityMode    uint32
	VideoOnConnect  int32
	AudioOnConnect  int32
}

func IsStartFrame(frameType uint8) bool {
	return frameType == FrameTypeStart || frameType == FrameTypeStartAlt
}

func IsEndFrame(frameType uint8) bool {
	return frameType == FrameTypeEndSingle ||
		frameType == FrameTypeEndMulti ||
		frameType == FrameTypeEndExt
}

func IsContinuationFrame(frameType uint8) bool {
	return frameType == FrameTypeCont || frameType == FrameTypeContAlt
}

type PacketHeader struct {
	Channel      byte
	FrameType    byte
	HeaderSize   int    // 28 or 36
	FrameNo      uint32 // Frame number (from [24-27] for 28-byte, [32-35] for 36-byte)
	PktIdx       uint16 // Packet index within frame (0-based)
	PktTotal     uint16 // Total packets in this frame
	PayloadSize  uint16
	HasFrameInfo bool // true if [14-15] or [22-23] == 0x0028
}

func ParsePacketHeader(data []byte) *PacketHeader {
	if len(data) < 28 {
		return nil
	}

	frameType := data[1]
	hdr := &PacketHeader{
		Channel:   data[0],
		FrameType: frameType,
	}

	// Header size based on FrameType (NOT magic bytes!)
	switch frameType {
	case FrameTypeStart, FrameTypeStartAlt, FrameTypeEndExt: // 0x08, 0x09, 0x0d
		hdr.HeaderSize = 36
	default: // 0x00, 0x01, 0x04, 0x05
		hdr.HeaderSize = 28
	}

	if len(data) < hdr.HeaderSize {
		return nil
	}

	if hdr.HeaderSize == 28 {
		// 28-Byte Header Layout:
		// [12-13] pkt_total
		// [14-15] pkt_idx OR 0x0028 (FrameInfo marker) - ONLY 0x0028 in End packets!
		// [16-17] payload_size
		// [24-27] frame_no (uint32)
		hdr.PktTotal = binary.LittleEndian.Uint16(data[12:])
		pktIdxOrMarker := binary.LittleEndian.Uint16(data[14:])
		hdr.PayloadSize = binary.LittleEndian.Uint16(data[16:])
		hdr.FrameNo = binary.LittleEndian.Uint32(data[24:])

		// 0x0028 is FrameInfo marker ONLY in End packets, otherwise it's pkt_idx=40
		if IsEndFrame(hdr.FrameType) && pktIdxOrMarker == 0x0028 {
			hdr.HasFrameInfo = true
			if hdr.PktTotal > 0 {
				hdr.PktIdx = hdr.PktTotal - 1 // Last packet
			}
		} else {
			hdr.PktIdx = pktIdxOrMarker
		}
	} else {
		// 36-Byte Header Layout:
		// [20-21] pkt_total
		// [22-23] pkt_idx OR 0x0028 (FrameInfo marker) - ONLY 0x0028 in End packets!
		// [24-25] payload_size
		// [32-35] frame_no (uint32) - GLOBAL frame counter, matches 28-byte [24-27]
		// NOTE: [18-19] is channel-specific frame index, NOT used for reassembly!
		hdr.PktTotal = binary.LittleEndian.Uint16(data[20:])
		pktIdxOrMarker := binary.LittleEndian.Uint16(data[22:])
		hdr.PayloadSize = binary.LittleEndian.Uint16(data[24:])
		hdr.FrameNo = binary.LittleEndian.Uint32(data[32:])

		// 0x0028 is FrameInfo marker ONLY in End packets, otherwise it's pkt_idx=40
		if IsEndFrame(hdr.FrameType) && pktIdxOrMarker == 0x0028 {
			hdr.HasFrameInfo = true
			if hdr.PktTotal > 0 {
				hdr.PktIdx = hdr.PktTotal - 1
			}
		} else {
			hdr.PktIdx = pktIdxOrMarker
		}
	}

	return hdr
}
