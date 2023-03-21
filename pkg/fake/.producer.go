package fake

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"time"
)

type Producer struct {
	streamer.Element
	Medias []*streamer.Media
	Tracks []*streamer.Track

	RecvPackets int
	SendPackets int
}

func (p *Producer) GetMedias() []*streamer.Media {
	return p.Medias
}

func (p *Producer) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	if !streamer.Contains(p.Medias, media, codec) {
		panic("you shall not pass!")
	}

	track := streamer.NewTrack(codec, media.Direction)

	switch media.Direction {
	case streamer.DirectionSendonly:
		track2 := track.Bind(func(packet *rtp.Packet) error {
			p.RecvPackets++
			return nil
		})
		p.Tracks = append(p.Tracks, track2)
	case streamer.DirectionRecvonly:
		p.Tracks = append(p.Tracks, track)
	}

	return track
}

func (p *Producer) Start() error {
	for {
		for _, track := range p.Tracks {
			if track.Direction != streamer.DirectionSendonly {
				continue
			}
			pkt := &rtp.Packet{}
			pkt.PayloadType = track.Codec.PayloadType
			if err := track.WriteRTP(pkt); err != nil {
				return err
			}
			p.SendPackets++
		}
		time.Sleep(time.Second)
	}
}

func (p *Producer) Stop() error {
	panic("not implemented")
}
