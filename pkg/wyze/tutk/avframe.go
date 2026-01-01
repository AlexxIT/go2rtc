package tutk

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/aac"
)

const FrameInfoSize = 40

// Wire format (little-endian) - Wyze extended FRAMEINFO:
//
//	[0-1]   codec_id    uint16  (0x004e=H.264, 0x0050=H.265, 0x0088=AAC)
//	[2]     flags       uint8   (Video: 0=P/1=I, Audio: sr_idx<<2|bits16<<1|ch)
//	[3]     cam_index   uint8
//	[4]     online_num  uint8
//	[5]     framerate   uint8   (FPS, e.g. 20)
//	[6]     frame_size  uint8   (Resolution: 1=1080P, 2=360P, 4=2K)
//	[7]     bitrate     uint8   (e.g. 0xF0=240)
//	[8-11]  timestamp_us uint32 (microseconds component)
//	[12-15] timestamp   uint32  (Unix timestamp in seconds)
//	[16-19] payload_sz  uint32  (frame payload size)
//	[20-23] frame_no    uint32  (frame number)
//	[24-39] device_id   16 bytes (MAC address + padding)
type FrameInfo struct {
	CodecID     uint16
	Flags       uint8
	CamIndex    uint8
	OnlineNum   uint8
	Framerate   uint8 // FPS (e.g. 20)
	FrameSize   uint8 // Resolution: 1=1080P, 2=360P, 4=2K
	Bitrate     uint8 // Bitrate value (e.g. 240)
	TimestampUS uint32
	Timestamp   uint32
	PayloadSize uint32
	FrameNo     uint32
}

// Resolution constants (as received in FrameSize field)
// Note: Some cameras only support 2K + 360P, others support 1080P + 360P
// The actual resolution depends on camera model!
const (
	ResolutionUnknown = 0
	ResolutionSD      = 1 // 360P (640x360) on 2K cameras, or 1080P on older cams
	Resolution360P    = 2 // 360P (640x360)
	Resolution2K      = 4 // 2K (2560x1440)
)

func (fi *FrameInfo) IsKeyframe() bool {
	return fi.Flags == 0x01
}

// Resolution returns a human-readable resolution string
func (fi *FrameInfo) Resolution() string {
	switch fi.FrameSize {
	case ResolutionSD:
		return "SD" // Could be 360P or 1080P depending on camera
	case Resolution360P:
		return "360P"
	case Resolution2K:
		return "2K"
	default:
		return "unknown"
	}
}

func (fi *FrameInfo) SampleRate() uint32 {
	srIdx := (fi.Flags >> 2) & 0x0F
	return uint32(SampleRateValue(srIdx))
}

func (fi *FrameInfo) Channels() uint8 {
	if fi.Flags&0x01 == 1 {
		return 2
	}
	return 1
}

func (fi *FrameInfo) IsVideo() bool {
	return IsVideoCodec(fi.CodecID)
}

func (fi *FrameInfo) IsAudio() bool {
	return IsAudioCodec(fi.CodecID)
}

func ParseFrameInfo(data []byte) *FrameInfo {
	if len(data) < FrameInfoSize {
		return nil
	}

	offset := len(data) - FrameInfoSize
	fi := data[offset:]

	return &FrameInfo{
		CodecID:     binary.LittleEndian.Uint16(fi[0:2]),
		Flags:       fi[2],
		CamIndex:    fi[3],
		OnlineNum:   fi[4],
		Framerate:   fi[5],
		FrameSize:   fi[6],
		Bitrate:     fi[7],
		TimestampUS: binary.LittleEndian.Uint32(fi[8:12]),
		Timestamp:   binary.LittleEndian.Uint32(fi[12:16]),
		PayloadSize: binary.LittleEndian.Uint32(fi[16:20]),
		FrameNo:     binary.LittleEndian.Uint32(fi[20:24]),
	}
}

func ParseAudioParams(payload []byte, fi *FrameInfo) (sampleRate uint32, channels uint8) {
	// Try ADTS header first (more reliable than FRAMEINFO flags)
	if aac.IsADTS(payload) {
		codec := aac.ADTSToCodec(payload)
		if codec != nil {
			return codec.ClockRate, codec.Channels
		}
	}

	// Fallback to FRAMEINFO flags
	if fi != nil {
		return fi.SampleRate(), fi.Channels()
	}

	// Default values
	return 16000, 1
}
