package flv

import (
	"bytes"
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/avc"
	"github.com/pion/rtp"
)

type Client struct {
	URL string

	rd io.Reader

	medias    []*core.Media
	receivers []*core.Receiver

	recv int
}

func NewClient(rd io.Reader) *Client {
	return &Client{rd: rd}
}

func (c *Client) Describe() error {
	if err := c.ReadHeader(); err != nil {
		return err
	}

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
		tagType, _, b, err := c.ReadTag()
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

			codec := avc.ConfigToCodec(b[5:])
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
	video, audio := core.VA(c.receivers)

	for {
		tagType, timeMS, b, err := c.ReadTag()
		if err != nil {
			return err
		}

		c.recv += len(b)

		switch tagType {
		case TagAudio:
			if audio == nil || b[1] == 0 {
				continue
			}

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Timestamp: TimeToRTP(timeMS, audio.Codec.ClockRate),
				},
				Payload: b[2:],
			}
			audio.WriteRTP(pkt)

		case TagVideo:
			// frame type 4b, codecID 4b, avc packet type 8b, composition time 24b
			if video == nil || b[1] == 0 {
				continue
			}

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Timestamp: TimeToRTP(timeMS, video.Codec.ClockRate),
				},
				Payload: b[5:],
			}
			video.WriteRTP(pkt)
		}
	}
}
