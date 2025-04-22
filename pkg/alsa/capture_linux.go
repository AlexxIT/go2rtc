package alsa

import (
	"github.com/AlexxIT/go2rtc/pkg/alsa/device"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

type Capture struct {
	core.Connection
	dev    *device.Device
	closed core.Waiter
}

func newCapture(dev *device.Device) (*Capture, error) {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCML, ClockRate: 16000},
			},
		},
	}
	return &Capture{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "alsa",
			Medias:     medias,
			Transport:  dev,
		},
		dev: dev,
	}, nil
}

func (c *Capture) Start() error {
	dst := c.Medias[0].Codecs[0]
	src := &core.Codec{
		Name:      dst.Name,
		ClockRate: c.dev.GetRateNear(dst.ClockRate),
		Channels:  c.dev.GetChannelsNear(dst.Channels),
	}

	if err := c.dev.SetHWParams(device.SNDRV_PCM_FORMAT_S16_LE, src.ClockRate, src.Channels); err != nil {
		return err
	}

	transcode := transcodeFunc(dst, src)
	frameBytes := int(pcm.BytesPerFrame(src))

	var ts uint32

	// readBufferSize for 20ms interval
	readBufferSize := 20 * frameBytes * int(src.ClockRate) / 1000
	b := make([]byte, readBufferSize)
	for {
		n, err := c.dev.Read(b)
		if err != nil {
			return err
		}

		c.Recv += n

		if len(c.Receivers) == 0 {
			continue
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:   2,
				Marker:    true,
				Timestamp: ts,
			},
			Payload: transcode(b[:n]),
		}
		c.Receivers[0].WriteRTP(pkt)

		ts += uint32(n / frameBytes)
	}
}

func transcodeFunc(dst, src *core.Codec) func([]byte) []byte {
	if dst.ClockRate == src.ClockRate && dst.Channels == src.Channels {
		return func(b []byte) []byte {
			return b
		}
	}
	return pcm.Transcode(dst, src)
}
