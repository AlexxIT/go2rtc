package mpegts

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"net/http"
)

type Client struct {
	streamer.Element

	medias []*streamer.Media
	tracks map[byte]*streamer.Track

	res *http.Response
}

func NewClient(res *http.Response) *Client {
	return &Client{res: res}
}

func (c *Client) Handle() error {
	if c.tracks == nil {
		c.tracks = map[byte]*streamer.Track{}
	}

	reader := NewReader()

	b := make([]byte, 1024*1024*256) // 256K

	probe := streamer.NewProbe(c.medias == nil)
	for probe == nil || probe.Active() {
		n, err := c.res.Body.Read(b)
		if err != nil {
			return err
		}

		reader.AppendBuffer(b[:n])

		for {
			packet := reader.GetPacket()
			if packet == nil {
				break
			}

			track := c.tracks[packet.PayloadType]
			if track == nil {
				// count track on probe state even if not support it
				probe.Append(packet.PayloadType)

				media := GetMedia(packet)
				if media == nil {
					continue // unsupported codec
				}

				track = streamer.NewTrack2(media, nil)

				c.medias = append(c.medias, media)
				c.tracks[packet.PayloadType] = track
			}

			_ = track.WriteRTP(packet)

			//log.Printf("[AVC] %v, len: %d, pts: %d ts: %10d", h264.Types(packet.Payload), len(packet.Payload), pkt.PTS, packet.Timestamp)
		}
	}

	return nil
}

func (c *Client) Close() error {
	_ = c.res.Body.Close()
	return nil
}
