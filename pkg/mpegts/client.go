package mpegts

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"net/http"
)

type Client struct {
	core.Listener

	medias    []*core.Media
	receivers []*core.Receiver

	res *http.Response

	recv int
}

func NewClient(res *http.Response) *Client {
	return &Client{res: res}
}

func (c *Client) Handle() error {
	reader := NewReader()

	b := make([]byte, 1024*256) // 256K

	probe := core.NewProbe(c.medias == nil)
	for probe == nil || probe.Active() {
		n, err := c.res.Body.Read(b)
		if err != nil {
			return err
		}

		c.recv += n

		reader.AppendBuffer(b[:n])

	reading:
		for {
			packet := reader.GetPacket()
			if packet == nil {
				break
			}

			for _, receiver := range c.receivers {
				if receiver.ID == packet.PayloadType {
					receiver.WriteRTP(packet)
					continue reading
				}
			}

			// count track on probe state even if not support it
			probe.Append(packet.PayloadType)

			media := GetMedia(packet)
			if media == nil {
				continue // unsupported codec
			}

			c.medias = append(c.medias, media)

			receiver := core.NewReceiver(media, media.Codecs[0])
			receiver.ID = packet.PayloadType
			c.receivers = append(c.receivers, receiver)

			receiver.WriteRTP(packet)

			//log.Printf("[AVC] %v, len: %d, pts: %d ts: %10d", h264.Types(packet.Payload), len(packet.Payload), pkt.PTS, packet.Timestamp)
		}
	}

	return nil
}

func (c *Client) Close() error {
	_ = c.res.Body.Close()
	return nil
}
