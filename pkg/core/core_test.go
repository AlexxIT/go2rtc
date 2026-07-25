package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type producer struct {
	Medias    []*Media
	Receivers []*Receiver

	id byte
}

func (p *producer) GetMedias() []*Media {
	return p.Medias
}

func (p *producer) GetTrack(_ *Media, codec *Codec) (*Receiver, error) {
	for _, receiver := range p.Receivers {
		if receiver.Codec == codec {
			return receiver, nil
		}
	}
	receiver := NewReceiver(nil, codec)
	p.Receivers = append(p.Receivers, receiver)
	return receiver, nil
}

func (p *producer) Start() error {
	pkt := &Packet{Payload: []byte{p.id}}
	p.Receivers[0].Input(pkt)
	return nil
}

func (p *producer) Stop() error {
	for _, receiver := range p.Receivers {
		receiver.Close()
	}
	return nil
}

type consumer struct {
	Medias  []*Media
	Senders []*Sender

	cache chan byte
}

func (c *consumer) GetMedias() []*Media {
	return c.Medias
}

func (c *consumer) AddTrack(_ *Media, _ *Codec, track *Receiver) error {
	c.cache = make(chan byte, 1)
	sender := NewSender(nil, track.Codec)
	sender.Output = func(packet *Packet) {
		c.cache <- packet.Payload[0]
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *consumer) Stop() error {
	for _, sender := range c.Senders {
		sender.Close()
	}
	return nil
}

func (c *consumer) read() byte {
	return <-c.cache
}

func TestName(t *testing.T) {
	GetProducer := func(b byte) Producer {
		return &producer{
			Medias: []*Media{
				{
					Kind:      KindVideo,
					Direction: DirectionRecvonly,
					Codecs: []*Codec{
						{Name: CodecH264},
					},
				},
			},
			id: b,
		}
	}

	// stage1
	prod1 := GetProducer(1)
	cons2 := &consumer{}

	media1 := prod1.GetMedias()[0]
	track1, _ := prod1.GetTrack(media1, media1.Codecs[0])

	_ = cons2.AddTrack(nil, nil, track1)

	_ = prod1.Start()
	require.Equal(t, byte(1), cons2.read())

	// stage2
	prod2 := GetProducer(2)
	media2 := prod2.GetMedias()[0]
	require.NotEqual(t, fmt.Sprintf("%p", media1), fmt.Sprintf("%p", media2))
	track2, _ := prod2.GetTrack(media2, media2.Codecs[0])
	track1.Replace(track2)

	_ = prod1.Stop()

	_ = prod2.Start()
	require.Equal(t, byte(2), cons2.read())

	// stage3
	_ = prod2.Stop()
}
