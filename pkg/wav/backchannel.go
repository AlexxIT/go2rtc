package wav

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/pion/rtp"
)

type Backchannel struct {
	core.Connection
	cmd *shell.Command
}

func NewBackchannel(cmd *shell.Command) (core.Producer, error) {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				//{Name: core.CodecPCML},
				{Name: core.CodecPCMA},
				{Name: core.CodecPCMU},
			},
		},
	}

	return &Backchannel{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "wav",
			Protocol:   "pipe",
			Medias:     medias,
			Transport:  cmd,
		},
		cmd: cmd,
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

	b := Header(track.Codec)
	if _, err = wr.Write(b); err != nil {
		return err
	}

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if n, err := wr.Write(packet.Payload); err != nil {
			c.Send += n
		}
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Backchannel) Start() error {
	return c.cmd.Run()
}
