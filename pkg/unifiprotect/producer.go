package unifiprotect

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

var opusConfig = []byte{0xcf, 0x00, 0x03, 0x02}

type AudioMode byte

const (
	AudioNone AudioMode = iota
	AudioAAC
	AudioOpus
)

type Producer struct {
	core.Connection

	rd        *Reader
	pending   []*Tag
	audioMode AudioMode
	audioType byte

	video *core.Receiver
	audio *core.Receiver

	audioSeq uint16
	started  time.Time
	onStop   func()
	stopOnce sync.Once
}

func Open(rd io.ReadCloser, audioMode AudioMode) (*Producer, error) {
	p := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "unifi-protect",
			Protocol:   "tcp",
			Transport:  rd,
		},
		rd:        NewReader(rd),
		audioMode: audioMode,
		started:   time.Now(),
	}
	if err := p.probe(); err != nil {
		_ = rd.Close()
		return nil, err
	}
	return p, nil
}

func (p *Producer) SetOnStop(fn func()) {
	p.onStop = fn
}

func (p *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	receiver, err := p.Connection.GetTrack(media, codec)
	if err != nil {
		return nil, err
	}
	if media.Kind == core.KindVideo {
		p.video = receiver
	} else {
		p.audio = receiver
	}
	return receiver, nil
}

func (p *Producer) Start() error {
	defer p.release()

	for _, tag := range p.pending {
		p.writeTag(tag)
	}
	p.pending = nil

	for {
		tag, err := p.rd.ReadTag()
		if err != nil {
			return err
		}
		p.writeTag(tag)
	}
}

func (p *Producer) Stop() error {
	p.release()
	return p.Connection.Stop()
}

func (p *Producer) release() {
	p.stopOnce.Do(func() {
		if p.onStop != nil {
			p.onStop()
		}
	})
}

func (p *Producer) probe() error {
	var video, opus, audio *core.Codec

	for {
		tag, err := p.rd.ReadTag()
		if err != nil {
			if video == nil {
				return fmt.Errorf("unifi-protect: probe video: %w", err)
			}
			break
		}
		p.pending = append(p.pending, tag)

		switch tag.Type {
		case TagVideo:
			if video == nil && isAVCConfig(tag.Data) {
				_, sps, pps := h264.DecodeConfig(tag.Data[5:])
				if len(sps) != 0 && len(pps) != 0 {
					video = h264.ConfigToCodec(tag.Data[5:])
				}
			}

		case TagOpus:
			if bytes.Equal(tag.Data, opusConfig) {
				opus = &core.Codec{
					Name:        core.CodecOpus,
					ClockRate:   48000,
					Channels:    2,
					FmtpLine:    "stereo=0;sprop-stereo=0",
					PayloadType: 111,
				}
			}

		case TagAudio:
			if audio == nil && isAACConfig(tag.Data) {
				codec := aac.ConfigToCodec(tag.Data[2:])
				if codec.Name == core.CodecAAC && codec.ClockRate != 0 {
					audio = codec
				}
			}
		}

		if video != nil && (p.audioMode == AudioNone || p.audioMode == AudioOpus && opus != nil || p.audioMode == AudioAAC && audio != nil) {
			break
		}
	}

	p.Medias = append(p.Medias, &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{video},
	})

	if p.audioMode == AudioNone {
		audio = nil
	} else if p.audioMode == AudioOpus && opus != nil || audio == nil && opus != nil {
		p.audioType = TagOpus
		audio = opus
	} else if audio != nil {
		p.audioType = TagAudio
	} else {
		audio = nil
	}
	if audio != nil {
		p.Medias = append(p.Medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{audio},
		})
	}

	return nil
}

func (p *Producer) writeTag(tag *Tag) {
	p.Recv += len(tag.Data)

	switch tag.Type {
	case TagVideo:
		if p.video == nil || len(tag.Data) < 5 || tag.Data[0]&0x0f != 7 || tag.Data[1] != 1 {
			return
		}
		p.video.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Marker:    true,
				Timestamp: p.timestamp(tag.Timestamp, p.video.Codec.ClockRate),
			},
			Payload: tag.Data[5:],
		})

	case TagOpus:
		if p.audio == nil || p.audioType != TagOpus || len(tag.Data) == 0 || bytes.Equal(tag.Data, opusConfig) {
			return
		}
		p.audioSeq++
		p.audio.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    p.audio.Codec.PayloadType,
				SequenceNumber: p.audioSeq,
				Timestamp:      p.timestamp(tag.Timestamp, p.audio.Codec.ClockRate),
			},
			Payload: tag.Data,
		})

	case TagAudio:
		if p.audio == nil || p.audioType != TagAudio || len(tag.Data) < 2 || tag.Data[0]>>4 != 10 || tag.Data[1] != 1 {
			return
		}
		p.audio.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Marker:    true,
				Timestamp: p.timestamp(tag.Timestamp, p.audio.Codec.ClockRate),
			},
			Payload: tag.Data[2:],
		})
	}
}

func (p *Producer) timestamp(ms, clockRate uint32) uint32 {
	if ms == ^uint32(0) {
		ms = uint32(time.Since(p.started) / time.Millisecond)
	}
	return uint32(uint64(ms) * uint64(clockRate) / 1000)
}

func isAVCConfig(data []byte) bool {
	return len(data) >= 6 && data[0]&0x0f == 7 && data[1] == 0
}

func isAACConfig(data []byte) bool {
	return len(data) >= 4 && data[0]>>4 == 10 && data[1] == 0
}

var _ core.Producer = (*Producer)(nil)
