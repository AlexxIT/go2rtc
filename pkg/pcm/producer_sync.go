package pcm

import (
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type ProducerSync struct {
	core.Connection
	src     *core.Codec
	rd      io.Reader
	onClose func()
}

func OpenSync(codec *core.Codec, rd io.Reader) *ProducerSync {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    ProducerCodecs(),
		},
	}

	return &ProducerSync{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "pcm",
			Medias:     medias,
			Transport:  rd,
		},
		src: codec,
		rd:  rd,
	}
}

func (p *ProducerSync) OnClose(f func()) {
	p.onClose = f
}

func (p *ProducerSync) Start() error {
	if len(p.Receivers) == 0 {
		return nil
	}

	var pktSeq uint16
	var pktTS uint32          // time in frames
	var pktTime time.Duration // time in seconds

	t0 := time.Now()

	dst := p.Receivers[0].Codec
	transcode := Transcode(dst, p.src)

	const chunkDuration = 20 * time.Millisecond
	chunkBytes := BytesPerDuration(p.src, chunkDuration)
	chunkFrames := uint32(FramesPerDuration(dst, chunkDuration))

	for {
		buf := make([]byte, chunkBytes)
		n, _ := io.ReadFull(p.rd, buf)

		if n == 0 {
			break
		}

		pkt := &core.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				SequenceNumber: pktSeq,
				Timestamp:      pktTS,
			},
			Payload: transcode(buf[:n]),
		}

		if d := pktTime - time.Since(t0); d > 0 {
			time.Sleep(d)
		}

		p.Receivers[0].WriteRTP(pkt)
		p.Recv += n

		pktSeq++
		pktTS += chunkFrames
		pktTime += chunkDuration
	}

	if p.onClose != nil {
		p.onClose()
	}

	return nil
}
