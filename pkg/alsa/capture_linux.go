package alsa

import (
	"github.com/AlexxIT/go2rtc/pkg/alsa/device"
	"github.com/AlexxIT/go2rtc/pkg/core"
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

// readBufferSize - 20ms * 2 bytes per sample * 16000 frames per second * 2 channels / 1000ms per second
const readBufferSize = 20 * 2 * 16000 * 2 / 1000

// bytesPerFrame - 2 bytes per sample * 2 channels
const bytesPerFrame = 2 * 2

func (c *Capture) Start() error {
	if err := c.dev.SetHWParams(device.SNDRV_PCM_FORMAT_S16_LE, 16000, 2); err != nil {
		return err
	}

	var ts uint32

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
			Payload: stereoToMono(b[:n]),
		}
		c.Receivers[0].WriteRTP(pkt)

		ts += uint32(n / bytesPerFrame)
	}
}

func stereoToMono(stereo []byte) (mono []byte) {
	n := len(stereo)
	mono = make([]byte, n/2)
	var i, j int
	for i < n {
		mono[j] = stereo[i]
		j++
		i++
		mono[j] = stereo[i]
		j++
		i += 3
	}
	return
}
