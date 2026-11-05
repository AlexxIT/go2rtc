package flv

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
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

	CodecH264 = 7
	CodecHEVC = 12
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
			if c.audio == nil || len(pkt.Payload) < 2 || pkt.Payload[1] == 0 {
				continue
			}

			pkt.Timestamp = TimeToRTP(pkt.Timestamp, c.audio.Codec.ClockRate)
			pkt.Payload = pkt.Payload[2:]
			c.audio.WriteRTP(pkt)

		case TagVideo:
			if c.video == nil || len(pkt.Payload) < 2 {
				continue
			}

			if isExHeader(pkt.Payload) {
				switch packetType := pkt.Payload[0] & 0b1111; packetType {
				case PacketTypeCodedFrames:
					if len(pkt.Payload) < 8 {
						continue
					}
					// frame type 4b, packet type 4b, fourCC 32b, composition time 24b
					pkt.Payload = pkt.Payload[8:]
				case PacketTypeCodedFramesX:
					if len(pkt.Payload) < 5 {
						continue
					}
					// frame type 4b, packet type 4b, fourCC 32b
					pkt.Payload = pkt.Payload[5:]
				default:
					continue
				}
			} else {
				switch pkt.Payload[1] {
				case PacketTypeAVCNALU:
					if len(pkt.Payload) < 5 {
						continue
					}
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

	waitVideo := true
	hasAudio := false
	timeout := time.Now().Add(time.Second * 5)

	for waitVideo && time.Now().Before(timeout) {
		pkt, err := c.readPacket()
		if err != nil {
			return err
		}

		switch pkt.PayloadType {
		case TagAudio:
			if hasAudio || len(pkt.Payload) < 2 {
				continue
			}

			codecID := pkt.Payload[0] >> 4 // SoundFormat
			if codecID == CodecAAC {
				var codec *core.Codec
				if pkt.Payload[1] == 0 {
					codec = aac.ConfigToCodec(pkt.Payload[2:])
				} else {
					codec = &core.Codec{
						Name:        core.CodecAAC,
						ClockRate:   44100,
						Channels:    2,
						PayloadType: core.PayloadTypeRAW,
					}
				}
				if codec != nil {
					media := &core.Media{
						Kind:      core.KindAudio,
						Direction: core.DirectionRecvonly,
						Codecs:    []*core.Codec{codec},
					}
					c.Medias = append(c.Medias, media)
					hasAudio = true
				}
			}

		case TagVideo:
			if !waitVideo || len(pkt.Payload) < 2 {
				continue
			}

			var codec *core.Codec

			if isExHeader(pkt.Payload) {
				if len(pkt.Payload) >= 5 && string(pkt.Payload[1:5]) == "hvc1" {
					codec = h265.ConfigToCodec(pkt.Payload[5:])
				}
			} else {
				packetType := pkt.Payload[1]
				codecID := pkt.Payload[0] & 0b1111

				if packetType == PacketTypeAVCHeader {
					switch codecID {
					case CodecH264:
						codec = h264.ConfigToCodec(pkt.Payload[5:])
					case CodecHEVC:
						codec = h265.ConfigToCodec(pkt.Payload[5:])
					}
				} else if packetType == PacketTypeAVCNALU {
					switch codecID {
					case CodecH264:
						c := h264.AVCCToCodec(pkt.Payload[5:])
						if c != nil && strings.Contains(c.FmtpLine, "sprop-parameter-sets") {
							codec = c
						}
					case CodecHEVC:
						codec = &core.Codec{
							Name:        core.CodecH265,
							ClockRate:   90000,
							PayloadType: core.PayloadTypeRAW,
						}
					}
				}
			}

			if codec == nil {
				codec = &core.Codec{
					Name:        core.CodecH264,
					ClockRate:   90000,
					FmtpLine:    "packetization-mode=1",
					PayloadType: core.PayloadTypeRAW,
				}
			}

			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)
			waitVideo = false

		case TagData:
			// No-op for onMetaData
		}
	}

	if len(c.Medias) == 0 {
		media := &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{{
				Name:        core.CodecH264,
				ClockRate:   90000,
				FmtpLine:    "packetization-mode=1",
				PayloadType: core.PayloadTypeRAW,
			}},
		}
		c.Medias = append(c.Medias, media)
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

// TimeToRTP convert time in milliseconds to RTP time
func TimeToRTP(timeMS, clockRate uint32) uint32 {
	// for clockRates 90000, 16000, 8000, etc. - we can use:
	//     return timeMS * (clockRate / 1000)
	// but for clockRates 44100, 22050, 11025 - we should use:
	return uint32(uint64(timeMS) * uint64(clockRate) / 1000)
}

func isExHeader(data []byte) bool {
	return data[0]&0b1000_0000 != 0
}
