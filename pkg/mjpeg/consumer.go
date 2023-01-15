package mjpeg

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"sync/atomic"
)

type Consumer struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	codecs []*streamer.Codec
	start  bool

	send uint32
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
		atomic.AddUint32(&c.send, uint32(len(packet.Payload)))
		return nil
	}

	if track.Codec.IsRTP() {
		wrapper := RTPDepay(track)
		push = wrapper(push)
	}

	return track.Bind(push)
}

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:       "MJPEG client",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Send:       atomic.LoadUint32(&c.send),
	}
	return json.Marshal(info)
}
