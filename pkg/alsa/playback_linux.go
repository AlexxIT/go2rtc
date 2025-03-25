package alsa

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/alsa/device"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

type Playback struct {
	core.Connection
	dev    *device.Device
	closed core.Waiter
}

func newPlayback(dev *device.Device) (*Playback, error) {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCML},                  // support ffmpeg producer (auto transcode)
				{Name: core.CodecPCMA, ClockRate: 8000}, // support webrtc producer
			},
		},
	}
	return &Playback{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "alsa",
			Medias:     medias,
			Transport:  dev,
		},
		dev: dev,
	}, nil
}

func (p *Playback) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (p *Playback) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	src := track.Codec

	// support probe
	if src.Name == core.CodecAny {
		src = &core.Codec{
			Name:      core.CodecPCML,
			ClockRate: 16000,
			Channels:  2,
		}
	}

	dst := &core.Codec{
		Name:      core.CodecPCML,
		ClockRate: src.ClockRate,
		Channels:  2,
	}
	sender := core.NewSender(media, dst)

	sender.Handler = func(pkt *rtp.Packet) {
		if n, err := p.dev.Write(pkt.Payload); err == nil {
			p.Send += n
		}
	}

	if sender.Handler = pcm.TranscodeHandler(dst, src, sender.Handler); sender.Handler == nil {
		return fmt.Errorf("alsa: can't convert %s to %s", src, dst)
	}

	// typical card support:
	// - Formats: S16_LE, S32_LE
	// - ClockRates: 8000 - 192000
	// - Channels: 2 - 10
	err := p.dev.SetHWParams(device.SNDRV_PCM_FORMAT_S16_LE, dst.ClockRate, 2)
	if err != nil {
		return err
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Playback) Start() (err error) {
	return p.closed.Wait()
}

func (p *Playback) Stop() error {
	p.closed.Done(nil)
	return p.Connection.Stop()
}
