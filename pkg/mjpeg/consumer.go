package mjpeg

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

type Consumer struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	codecs []*streamer.Codec
	start  bool

	send int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	return []*streamer.Media{{
		Kind:      streamer.KindVideo,
		Direction: streamer.DirectionRecvonly,
		Codecs:    []*streamer.Codec{{Name: streamer.CodecJPEG}},
	}}
}

func (c *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	push := func(packet *rtp.Packet) error {
		c.Fire(packet.Payload)
		return nil
	}

	if track.Codec.IsRTP() {
		wrapper := RTPDepay(track)
		push = wrapper(push)
	}

	return track.Bind(push)
}
