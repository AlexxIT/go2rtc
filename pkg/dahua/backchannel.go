package dahua

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	log := app.GetLogger("dahua")
	log.Debug().
		Str("media_kind", string(media.Kind)).
		Str("codec", track.Codec.Name).
		Msg("[dahua] adding track")

	if c.sender == nil {
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			// Get connection in a thread-safe way
			conn := c.getConnection()
			if conn == nil {
				return
			}

			// Send multipart boundary with payload
			boundary := "\r\n--go2rtc-audio-boundary\r\n"

			// Determine content type based on codec
			var contentType string
			switch track.Codec.Name {
			case core.CodecPCMA:
				contentType = "Audio/G.711A"
			case core.CodecPCMU:
				contentType = "Audio/G.711Mu"
			default:
				contentType = "Audio/G.711A"
			}

			boundary += "Content-Type: " + contentType + "\r\n"
			boundary += "Content-Length: " + fmt.Sprintf("%d", len(packet.Payload)) + "\r\n\r\n"

			// Write boundary first
			if _, err := conn.Write([]byte(boundary)); err != nil {
				log.Error().Err(err).Msg("[dahua] failed to write boundary")
				return
			}

			// Write payload
			n, err := conn.Write(packet.Payload)
			if err != nil {
				log.Error().Err(err).Msg("[dahua] failed to write audio payload")
				return
			}

			c.send += n
		}

		log.Debug().
			Str("codec", track.Codec.Name).
			Uint32("clock_rate", track.Codec.ClockRate).
			Msg("[dahua] audio sender created")
	}

	c.sender.HandleRTP(track)
	return nil
}

// getConnection returns the current connection in a thread-safe way
func (c *Client) getConnection() io.WriteCloser {
	// For now, just return the connection directly
	// TODO: Add proper synchronization if needed
	return c.conn
}

func (c *Client) Start() (err error) {
	log := app.GetLogger("dahua")
	log.Debug().Msg("[dahua] starting client")

	if err = c.open(); err != nil {
		log.Error().Err(err).Msg("[dahua] failed to open connection")
		return
	}

	log.Debug().Msg("[dahua] client started successfully")
	return
}

func (c *Client) Stop() (err error) {
	log := app.GetLogger("dahua")
	log.Debug().Msg("[dahua] stopping client")

	if c.sender != nil {
		c.sender.Close()
		log.Debug().Msg("[dahua] sender closed")
	}

	if c.conn != nil {
		err = c.close()
		if err != nil {
			log.Error().Err(err).Msg("[dahua] error during close")
		}
		// Don't call c.conn.Close() again since c.close() already handles it
		log.Debug().Msg("[dahua] connection closed")
	}

	log.Debug().Msg("[dahua] client stopped")
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.NewID(),
		FormatName: "dahua",
		Protocol:   "http",
		Medias:     c.medias,
		Send:       c.send,
	}
	if c.conn != nil {
		info.RemoteAddr = c.url // Use URL instead of RemoteAddr since we don't have a net.Conn
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}
