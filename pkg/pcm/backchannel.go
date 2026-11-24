package pcm

import (
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/pion/rtp"
)

type Backchannel struct {
	core.Connection
	cmd   *shell.Command
	codec *core.Codec
}

func NewBackchannel(cmd *shell.Command, audio string) (core.Producer, error) {
	var codec *core.Codec

	if audio == "" {
		// default codec
		codec = &core.Codec{Name: core.CodecPCML, ClockRate: 16000}
	} else if codec = core.ParseCodecString(audio); codec == nil {
		return nil, errors.New("pcm: unsupported audio format: " + audio)
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			// native codec first, then any PCM codec that can be transcoded to it
			Codecs: append([]*core.Codec{codec}, ConsumerCodecs()...),
		},
	}

	return &Backchannel{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "pcm",
			Protocol:   "pipe",
			Medias:     medias,
			Transport:  cmd,
		},
		cmd:   cmd,
		codec: codec,
	}, nil
}

func (c *Backchannel) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Backchannel) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	wr, err := c.cmd.StdinPipe()
	if err != nil {
		return err
	}

	handler := func(packet *rtp.Packet) {
		if n, err := wr.Write(packet.Payload); err == nil {
			c.Send += n
		}
	}

	src := track.Codec
	dst := *c.codec
	if dst.ClockRate == 0 {
		dst.ClockRate = src.ClockRate // ex. `#audio=pcmu` without sample rate
	}
	if src.Name != dst.Name || src.ClockRate != dst.ClockRate {
		handler = TranscodeHandler(&dst, src, handler)
	}

	sender := core.NewSender(media, track.Codec)
	sender.Handler = handler
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Backchannel) Start() error {
	return c.cmd.Run()
}
