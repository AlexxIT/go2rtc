package fake

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"time"
)

type Consumer struct {
	streamer.Element
	Medias []*streamer.Media
	Tracks []*streamer.Track

	RecvPackets int
	SendPackets int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	return c.Medias
}

func (c *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	switch track.Direction {
	case streamer.DirectionSendonly:
		track = track.Bind(func(packet *rtp.Packet) error {
			if track.Codec.PayloadType != packet.PayloadType {
				panic("wrong payload type")
			}
			c.RecvPackets++
			return nil
		})
	case streamer.DirectionRecvonly:
		go func() {
			for {
				pkt := &rtp.Packet{}
				pkt.PayloadType = track.Codec.PayloadType
				if err := track.WriteRTP(pkt); err != nil {
					return
				}
				c.SendPackets++
				time.Sleep(time.Second)
			}
		}()
	}
	c.Tracks = append(c.Tracks, track)
	return track
}
