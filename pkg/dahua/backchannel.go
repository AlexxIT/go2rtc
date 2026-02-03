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

		// Set up the audio handler
		c.sender.Handler = func(packet *rtp.Packet) {
			// Get connection in a thread-safe way
			conn := c.getConnection()
			if conn == nil {
				return
			}

			// For G.711, we should send raw audio data without RTP headers
			// The Dahua camera expects pure G.711 audio samples
			audioData := packet.Payload

			// Skip empty packets
			if len(audioData) == 0 {
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
			boundary += fmt.Sprintf("Content-Length: %d\r\n\r\n", len(audioData))

			// Write boundary and audio data in one operation to reduce fragmentation
			fullData := append([]byte(boundary), audioData...)

			// Use thread-safe connection writing
			c.connMu.Lock()
			n, err := conn.Write(fullData)
			c.connMu.Unlock()

			if err != nil {
				log.Debug().Err(err).Msg("[dahua] failed to write audio data")
				return
			}

			c.send += n
		}

		log.Debug().
			Str("codec", track.Codec.Name).
			Uint32("clock_rate", track.Codec.ClockRate).
			Msg("[dahua] audio sender created")
	}

	c.sender.WithParent(track).Start()
	return nil
}

// getConnection returns the current connection in a thread-safe way
func (c *Client) getConnection() io.WriteCloser {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
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
