package dahua

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

// Client implements Dahua CGI HTTP API for 2-way audio
type Client struct {
	core.Listener

	url      string
	username string
	password string
	channel  string
	conn     io.WriteCloser
	connMu   sync.RWMutex

	medias []*core.Media
	sender *core.Sender
	send   int
}

// Dial creates a new Dahua CGI client
func Dial(rawURL string) (*Client, error) {
	log := app.GetLogger("dahua")
	log.Debug().Str("url", rawURL).Msg("[dahua] creating new client")

	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Error().Err(err).Str("url", rawURL).Msg("[dahua] failed to parse URL")
		return nil, err
	}

	// Extract credentials
	username := u.User.Username()
	password, _ := u.User.Password()

	// Build base URL
	u.User = nil
	u.Scheme = "http"
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""

	client := &Client{
		url:      u.String(),
		username: username,
		password: password,
		channel:  "1", // Default channel
	}

	log.Debug().
		Str("url", client.url).
		Str("username", username).
		Str("channel", client.channel).
		Msg("[dahua] client configuration")

	if err = client.dial(); err != nil {
		log.Error().Err(err).Msg("[dahua] failed to dial")
		return nil, err
	}

	return client, nil
}

// dial establishes connection for audio output only (postAudio)
func (c *Client) dial() error {
	log := app.GetLogger("dahua")
	log.Debug().Msg("[dahua] starting dial process")

	// Create media descriptor for audio output (send-only from go2rtc perspective)
	// We support both G.711 A-law and μ-law for postAudio functionality
	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000}, // G.711 A-law
			{Name: core.CodecPCMU, ClockRate: 8000}, // G.711 μ-law
		},
	}
	c.medias = append(c.medias, media)

	log.Debug().
		Str("kind", string(media.Kind)).
		Str("direction", string(media.Direction)).
		Int("codecs", len(media.Codecs)).
		Msg("[dahua] media configuration created for postAudio")

	return nil
}

// open establishes the audio streaming connection
func (c *Client) open() error {
	log := app.GetLogger("dahua")
	log.Debug().Msg("[dahua] opening audio connection")

	// Determine content type based on codec
	var contentType string
	if c.sender != nil && c.sender.Codec != nil {
		switch c.sender.Codec.Name {
		case core.CodecPCMA:
			contentType = "Audio/G.711A"
		case core.CodecPCMU:
			contentType = "Audio/G.711Mu"
		default:
			contentType = "Audio/G.711A" // Default to A-law
		}
	} else {
		contentType = "Audio/G.711A" // Default to A-law
	}

	log.Debug().
		Str("content_type", contentType).
		Str("channel", c.channel).
		Msg("[dahua] opening multipart audio stream")

	// Use multipart POST for audio streaming
	link := fmt.Sprintf("%s/cgi-bin/audio.cgi?action=postAudio&httptype=multipart&channel=%s", c.url, c.channel)

	// Create request with a pipe for streaming body
	pipeReader, pipeWriter := io.Pipe()
	req, err := http.NewRequest("POST", link, pipeReader)
	if err != nil {
		return err
	}

	// Set credentials in URL for digest auth support
	req.URL.User = url.UserPassword(c.username, c.password)
	req.Header.Set("Content-Type", "multipart/x-mixed-replace; boundary=go2rtc-audio-boundary")
	req.Header.Set("Transfer-Encoding", "chunked")

	log.Debug().Str("url", link).Msg("[dahua] sending audio stream request")

	// Start the HTTP request in a goroutine
	go func() {
		defer pipeWriter.Close()

		res, err := tcp.Do(req)
		if err != nil {
			log.Error().Err(err).Msg("[dahua] failed to start audio stream")
			return
		}
		defer tcp.Close(res)

		// Log detailed response information
		log.Debug().
			Int("status", res.StatusCode).
			Str("status_text", res.Status).
			Str("content_type", res.Header.Get("Content-Type")).
			Str("content_length", res.Header.Get("Content-Length")).
			Str("connection", res.Header.Get("Connection")).
			Str("transfer_encoding", res.Header.Get("Transfer-Encoding")).
			Msg("[dahua] HTTP response details")

		if res.StatusCode != http.StatusOK {
			log.Error().
				Int("status", res.StatusCode).
				Str("status_text", res.Status).
				Msg("[dahua] audio stream request failed")
			return
		}

		log.Debug().Msg("[dahua] audio connection established successfully")

		// Read any response data
		buf := make([]byte, 1024)
		for {
			_, err := res.Body.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Debug().Err(err).Msg("[dahua] connection read error")
				}
				break
			}
		}
	}()

	// Store the pipe writer as our connection
	c.connMu.Lock()
	c.conn = pipeWriter
	c.connMu.Unlock()

	// Send initial multipart boundary
	boundary := "\r\n--go2rtc-audio-boundary\r\n"
	boundary += fmt.Sprintf("Content-Type: %s\r\n", contentType)
	boundary += "Content-Length: 0\r\n\r\n"

	if _, err := c.conn.Write([]byte(boundary)); err != nil {
		log.Error().Err(err).Msg("[dahua] failed to send initial boundary")
		return err
	}

	log.Debug().Msg("[dahua] initial boundary sent successfully")

	return nil
}

// close closes the audio connection
func (c *Client) close() error {
	log := app.GetLogger("dahua")
	log.Debug().Msg("[dahua] closing audio connection")

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		if err != nil {
			log.Error().Err(err).Msg("[dahua] error closing connection")
			return err
		}
	}

	log.Debug().Msg("[dahua] audio connection closed")
	return nil
}
