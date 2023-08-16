package flv

import (
	"bytes"
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

type Client struct {
	Transport

	URL string

	medias    []*core.Media
	receivers []*core.Receiver

	video, audio *core.Receiver

	recv int
}

func NewClient(rd io.Reader) (*Client, error) {
	tr, err := NewTransport(rd)
	if err != nil {
		return nil, err
	}
	return &Client{Transport: tr}, nil
}

func (c *Client) Describe() error {
	// Normal software sends:
	// 1. Video/audio flag in header
	// 2. MetaData as first tag (with video/audio codec info)
	// 3. Video/audio headers in 2nd and 3rd tag

	// Reolink camera sends:
	// 1. Empty video/audio flag
	// 2. MedaData without stereo key for AAC
	// 3. Audio header after Video keyframe tag
	waitVideo := true
	waitAudio := true
	timeout := time.Now().Add(core.ProbeTimeout)

	for (waitVideo || waitAudio) && time.Now().Before(timeout) {
		tagType, _, b, err := c.Transport.ReadTag()
		if err != nil {
			return err
		}

		c.recv += len(b)

		switch tagType {
		case TagAudio:
			if !waitAudio {
				continue
			}

			waitAudio = false

			codecID := b[0] >> 4 // SoundFormat
			_ = b[0] & 0b1100    // SoundRate
			_ = b[0] & 0b0010    // SoundSize
			_ = b[0] & 0b0001    // SoundType

			if codecID != CodecAAC {
				continue
			}

			if b[1] != 0 { // check if header
				continue
			}

			codec := aac.ConfigToCodec(b[2:])
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)

		case TagVideo:
			if !waitVideo {
				continue
			}

			waitVideo = false

			_ = b[0] >> 4            // FrameType
			codecID := b[0] & 0b1111 // CodecID

			if codecID != CodecAVC {
				continue
			}

			if b[1] != 0 { // check if header
				continue
			}

			codec := h264.ConfigToCodec(b[5:])
			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)

		case TagData:
			if !bytes.Contains(b, []byte("onMetaData")) {
				continue
			}
			waitVideo = bytes.Contains(b, []byte("videocodecid"))
			waitAudio = bytes.Contains(b, []byte("audiocodecid"))
		}
	}

	return nil
}

func (c *Client) Play() error {
	for {
		tagType, timeMS, b, err := c.Transport.ReadTag()
		if err != nil {
			return err
		}

		c.recv += len(b)

		switch tagType {
		case TagAudio:
			if c.audio == nil || b[1] == 0 {
				continue
			}

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Timestamp: TimeToRTP(timeMS, c.audio.Codec.ClockRate),
				},
				Payload: b[2:],
			}
			c.audio.WriteRTP(pkt)

		case TagVideo:
			// frame type 4b, codecID 4b, avc packet type 8b, composition time 24b
			if c.video == nil || b[1] == 0 {
				continue
			}

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Timestamp: TimeToRTP(timeMS, c.video.Codec.ClockRate),
				},
				Payload: b[5:],
			}
			c.video.WriteRTP(pkt)
		}
	}
}
