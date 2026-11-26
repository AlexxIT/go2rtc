package aac

import (
	"bufio"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// LOAS (AudioSyncStream, ISO/IEC 14496-3, 1.7.2) is a self-synchronising AAC
// stream like ADTS, but it can carry any audio object type. That makes it
// the stream format for AAC-ELD, which ADTS can't signal. FFmpeg writes it
// with "-f latm", the FDK AAC library with the LOAS transport type.
//
// Supported: one program, one layer, one subframe per frame (as written by
// FFmpeg and FDK AAC), with AAC-LC or AAC-LD/ELD without extensions.

func IsLOAS(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x56 && b[1]&0xE0 == 0xE0
}

type LOASProducer struct {
	core.Connection
	rd *bufio.Reader

	config  *loasConfig
	pending []byte // payload of the first frame, read by OpenLOAS
}

func OpenLOAS(r io.Reader) (*LOASProducer, error) {
	c := &LOASProducer{rd: bufio.NewReader(r)}

	// the first frame must carry the stream configuration
	payload, err := c.readFrame()
	if err != nil {
		return nil, err
	}
	if c.config == nil {
		return nil, errors.New("loas: no stream config")
	}
	c.pending = payload

	codec := ConfigToCodec(c.config.asc)
	if c.config.objType == TypeAACELD {
		codec.Name = core.CodecELD
	}

	c.Connection = core.Connection{
		ID:         core.NewID(),
		FormatName: "loas",
		Medias: []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			},
		},
		Transport: r,
	}
	return c, nil
}

func (c *LOASProducer) Start() error {
	payload := c.pending
	c.pending = nil

	for {
		if payload != nil {
			c.Recv += len(payload)

			if len(c.Receivers) > 0 {
				c.Receivers[0].WriteRTP(&rtp.Packet{
					Header:  rtp.Header{Timestamp: core.Now90000()},
					Payload: payload,
				})
			}
		}

		var err error
		if payload, err = c.readFrame(); err != nil {
			return err
		}
	}
}

// readFrame returns the access unit of the next frame, or nil for a frame
// that can't be decoded yet (no stream config received).
func (c *LOASProducer) readFrame() ([]byte, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(c.rd, header); err != nil {
		return nil, err
	}
	if !IsLOAS(header) {
		return nil, errors.New("loas: wrong sync")
	}

	frame := make([]byte, uint16(header[1]&0x1F)<<8|uint16(header[2]))
	if _, err := io.ReadFull(c.rd, frame); err != nil {
		return nil, err
	}

	rd := bits.NewReader(frame)

	// AudioMuxElement(muxConfigPresent = 1)
	if rd.ReadBit() == 0 { // useSameStreamMux
		config, err := readStreamMuxConfig(rd)
		if err != nil {
			return nil, err
		}
		c.config = config
	} else if c.config == nil {
		return nil, nil
	}

	// PayloadLengthInfo, frameLengthType 0
	var size int
	for {
		b := rd.ReadByte()
		size += int(b)
		if b != 255 {
			break
		}
	}

	// PayloadMux: the access unit, not necessarily byte aligned
	payload := rd.ReadBytes(size)
	if rd.EOF {
		return nil, errors.New("loas: short frame")
	}
	return payload, nil
}

type loasConfig struct {
	objType byte
	asc     []byte // AudioSpecificConfig
}

func readStreamMuxConfig(rd *bits.Reader) (*loasConfig, error) {
	audioMuxVersion := rd.ReadBit()
	if audioMuxVersion == 1 {
		if rd.ReadBit() != 0 { // audioMuxVersionA
			return nil, errors.New("loas: unsupported mux version")
		}
		_ = latmValue(rd) // taraBufferFullness
	}

	_ = rd.ReadBit() // allStreamsSameTimeFraming
	if rd.ReadBits8(6) != 0 || rd.ReadBits8(4) != 0 || rd.ReadBits8(3) != 0 {
		return nil, errors.New("loas: unsupported subframes, programs or layers")
	}

	var ascLen uint32
	if audioMuxVersion == 1 {
		ascLen = latmValue(rd)
	}

	config, used, err := readAudioSpecificConfig(rd)
	if err != nil {
		return nil, err
	}
	if ascLen > used {
		_ = rd.ReadBits(byte(ascLen - used)) // fill bits
	}

	if rd.ReadBits8(3) != 0 { // frameLengthType
		return nil, errors.New("loas: unsupported frame length type")
	}
	_ = rd.ReadByte() // latmBufferFullness

	if rd.ReadBit() == 1 { // otherDataPresent
		if audioMuxVersion == 1 {
			_ = latmValue(rd)
		} else {
			for {
				esc := rd.ReadBit()
				_ = rd.ReadByte()
				if esc == 0 {
					break
				}
			}
		}
	}

	if rd.ReadBit() == 1 { // crcCheckPresent
		_ = rd.ReadByte()
	}

	if rd.EOF {
		return nil, errors.New("loas: short stream config")
	}
	return config, nil
}

// readAudioSpecificConfig reads the config and returns it rebuilt with
// EncodeConfig, plus the number of bits read.
func readAudioSpecificConfig(rd *bits.Reader) (*loasConfig, uint32, error) {
	used := uint32(5 + 4 + 4)

	objType := rd.ReadBits8(5)
	if objType == TypeESCAPE {
		objType = 32 + rd.ReadBits8(6)
		used += 6
	}

	var sampleRate uint32
	if i := rd.ReadBits8(4); i < 0x0F {
		sampleRate = sampleRates[i]
	} else {
		sampleRate = rd.ReadBits(24)
		used += 24
	}

	channels := rd.ReadBits8(4)

	var shortFrame bool
	switch objType {
	case TypeAACLC:
		shortFrame = rd.ReadBit() == 1
		if rd.ReadBit() != 0 || rd.ReadBit() != 0 { // dependsOnCoreCoder, extensionFlag
			return nil, 0, errors.New("loas: unsupported AAC config")
		}
		used += 3
	case TypeAACLD:
		shortFrame = rd.ReadBit() == 1
		if rd.ReadBit() != 0 || rd.ReadBit() != 0 {
			return nil, 0, errors.New("loas: unsupported AAC-LD config")
		}
		_ = rd.ReadBits8(2) // epConfig
		used += 5
	case TypeAACELD:
		shortFrame = rd.ReadBit() == 1
		_ = rd.ReadBits8(3)                            // resilience flags
		if rd.ReadBit() != 0 || rd.ReadBits8(4) != 0 { // ldSbrPresentFlag, eldExtType
			return nil, 0, errors.New("loas: unsupported AAC-ELD config")
		}
		_ = rd.ReadBits8(2) // epConfig
		used += 11
	default:
		return nil, 0, errors.New("loas: unsupported audio object type")
	}

	return &loasConfig{
		objType: objType,
		asc:     EncodeConfig(objType, sampleRate, channels, shortFrame),
	}, used, nil
}

func latmValue(rd *bits.Reader) (v uint32) {
	n := rd.ReadBits8(2)
	for i := byte(0); i <= n; i++ {
		v = v<<8 | uint32(rd.ReadByte())
	}
	return
}
