package flv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *core.ReadBuffer

	video, audio *core.Receiver
}

func Open(rd io.Reader) (*Producer, error) {
	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "flv",
			Transport:  rd,
		},
		rd: core.NewReadBuffer(rd),
	}
	if err := prod.probe(); err != nil {
		return nil, err
	}
	return prod, nil
}

const (
	Signature = "FLV"

	TagAudio = 8
	TagVideo = 9
	TagData  = 18

	CodecAAC = 10
	CodecAVC = 7
)

const (
	PacketTypeAVCHeader = iota
	PacketTypeAVCNALU
	PacketTypeAVCEnd
)

const (
	PacketTypeSequenceStart = iota
	PacketTypeCodedFrames
	PacketTypeSequenceEnd
	PacketTypeCodedFramesX
	PacketTypeMetadata
	PacketTypeMPEG2TSSequenceStart
)

func (c *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	receiver, _ := c.Connection.GetTrack(media, codec)
	if media.Kind == core.KindVideo {
		c.video = receiver
	} else {
		c.audio = receiver
	}
	return receiver, nil
}

func (c *Producer) Start() error {
	for {
		pkt, err := c.readPacket()
		if err != nil {
			return err
		}

		c.Recv += len(pkt.Payload)

		switch pkt.PayloadType {
		case TagAudio:
			if c.audio == nil || pkt.Payload[1] == 0 {
				continue
			}

			pkt.Timestamp = TimeToRTP(pkt.Timestamp, c.audio.Codec.ClockRate)
			pkt.Payload = pkt.Payload[2:]
			c.audio.WriteRTP(pkt)

		case TagVideo:
			if c.video == nil {
				continue
			}

			if isExHeader(pkt.Payload) {
				switch packetType := pkt.Payload[0] & 0b1111; packetType {
				case PacketTypeCodedFrames:
					// frame type 4b, packet type 4b, fourCC 32b, composition time 24b
					pkt.Payload = pkt.Payload[8:]
				case PacketTypeCodedFramesX:
					// frame type 4b, packet type 4b, fourCC 32b
					pkt.Payload = pkt.Payload[5:]
				default:
					continue
				}
			} else {
				switch pkt.Payload[1] {
				case PacketTypeAVCNALU:
					// frame type 4b, codecID 4b, avc packet type 8b, composition time 24b
					pkt.Payload = pkt.Payload[5:]
				default:
					continue
				}
			}

			pkt.Timestamp = TimeToRTP(pkt.Timestamp, c.video.Codec.ClockRate)
			c.video.WriteRTP(pkt)
		}
	}
}

func (c *Producer) probe() error {
	if err := c.readHeader(); err != nil {
		return err
	}

	c.rd.BufferSize = core.ProbeSize
	defer c.rd.Reset()

	// Normal software sends:
	// 1. Video/audio flag in header
	// 2. MetaData as first tag (with video/audio codec info)
	// 3. Video/audio headers in 2nd and 3rd tag

	// Reolink camera sends:
	// 1. Empty video/audio flag
	// 2. MedaData without stereo key for AAC
	// 3. Audio header after Video keyframe tag
	waitType := []byte{TagData}
	timeout := time.Now().Add(core.ProbeTimeout)

	for len(waitType) != 0 && time.Now().Before(timeout) {
		pkt, err := c.readPacket()
		if err != nil {
			return err
		}

		if i := bytes.IndexByte(waitType, pkt.PayloadType); i < 0 {
			continue
		} else {
			waitType = append(waitType[:i], waitType[i+1:]...)
		}

		switch pkt.PayloadType {
		case TagAudio:
			_ = pkt.Payload[1] // bounds

			codecID := pkt.Payload[0] >> 4 // SoundFormat
			_ = pkt.Payload[0] & 0b1100    // SoundRate
			_ = pkt.Payload[0] & 0b0010    // SoundSize
			_ = pkt.Payload[0] & 0b0001    // SoundType

			if codecID != CodecAAC {
				continue
			}

			if pkt.Payload[1] != 0 { // check if header
				continue
			}

			codec := aac.ConfigToCodec(pkt.Payload[2:])
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)

		case TagVideo:
			var codec *core.Codec

			if isExHeader(pkt.Payload) {
				if string(pkt.Payload[1:5]) != "hvc1" {
					continue
				}

				if packetType := pkt.Payload[0] & 0b1111; packetType != PacketTypeSequenceStart {
					continue
				}

				codec = h265.ConfigToCodec(pkt.Payload[5:])
			} else {
				_ = pkt.Payload[0] >> 4 // FrameType

				if codecID := pkt.Payload[0] & 0b1111; codecID != CodecAVC {
					continue
				}

				if packetType := pkt.Payload[1]; packetType != PacketTypeAVCHeader { // check if header
					continue
				}

				codec = h264.ConfigToCodec(pkt.Payload[5:])
			}

			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)

		case TagData:
			if !bytes.Contains(pkt.Payload, []byte("onMetaData")) {
				waitType = append(waitType, TagData)
			}
			// Dahua cameras doesn't send videocodecid
			if bytes.Contains(pkt.Payload, []byte("videocodecid")) ||
				bytes.Contains(pkt.Payload, []byte("width")) ||
				bytes.Contains(pkt.Payload, []byte("framerate")) {
				waitType = append(waitType, TagVideo)
			}
			if bytes.Contains(pkt.Payload, []byte("audiocodecid")) {
				waitType = append(waitType, TagAudio)
			}
		}
	}

	return nil
}

func (c *Producer) readHeader() error {
	b := make([]byte, 9)
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}

	if string(b[:3]) != Signature {
		return errors.New("flv: wrong header")
	}

	_ = b[4] // flags (skip because unsupported by Reolink cameras)

	if skip := binary.BigEndian.Uint32(b[5:]) - 9; skip > 0 {
		if _, err := io.ReadFull(c.rd, make([]byte, skip)); err != nil {
			return err
		}
	}

	return nil
}

func (c *Producer) readPacket() (*rtp.Packet, error) {
	// https://rtmp.veriskope.com/pdf/video_file_format_spec_v10.pdf
	b := make([]byte, 4+11)
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return nil, err
	}

	b = b[4 : 4+11] // skip previous tag size

	size := uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])

	pkt := &rtp.Packet{
		Header: rtp.Header{
			PayloadType: b[0],
			Timestamp:   uint32(b[4])<<16 | uint32(b[5])<<8 | uint32(b[6]) | uint32(b[7])<<24,
		},
		Payload: make([]byte, size),
	}

	if _, err := io.ReadFull(c.rd, pkt.Payload); err != nil {
		return nil, err
	}

	//log.Printf("[FLV] %d %.40x", pkt.PayloadType, pkt.Payload)

	return pkt, nil
}

func TimeToRTP(timeMS uint32, clockRate uint32) uint32 {
	return timeMS * clockRate / 1000
}

func isExHeader(data []byte) bool {
	return data[0]&0b1000_0000 != 0
}
