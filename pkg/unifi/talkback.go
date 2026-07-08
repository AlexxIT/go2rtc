package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

const SchemeTalkback = "unifi-talkback"

var errStopped = errors.New("unifi: stopped")

type Client struct {
	core.Connection

	baseURL  *url.URL
	cameraID string
	apiKey   string

	ctx    context.Context
	cancel context.CancelFunc
	do     func(*http.Request) (*http.Response, error)

	mu          sync.Mutex
	stopped     bool
	active      *talkbackOutput
	activeOnce  sync.Once
	activeReady chan struct{}

	done     chan struct{}
	doneOnce sync.Once
	err      error
}

type publicCamera struct {
	FeatureFlags struct {
		HasSpeaker bool `json:"hasSpeaker"`
	} `json:"featureFlags"`
}

type TalkbackSession struct {
	URL           string `json:"url"`
	Codec         string `json:"codec"`
	SamplingRate  int    `json:"samplingRate"`
	BitsPerSample int    `json:"bitsPerSample,omitempty"`
}

func DialTalkback(rawURL string) (*Client, error) {
	innerURL := strings.TrimPrefix(rawURL, SchemeTalkback+":")
	if innerURL == rawURL {
		return nil, fmt.Errorf("unifi: unsupported scheme: %s", rawURL)
	}

	u, err := url.Parse(innerURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	cameraID := query.Get("camera_id")
	apiKey := query.Get("api_key")
	if cameraID == "" {
		return nil, errors.New("unifi: camera_id required")
	}
	if apiKey == "" {
		return nil, errors.New("unifi: api_key required")
	}

	RegisterTalkbackSecrets(rawURL)
	log.Debug().
		Str("url", creds.SecretString(rawURL)).
		Str("camera_id", cameraID).
		Msg("[unifi] dial talkback")

	u.RawQuery = ""
	u.Fragment = ""

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "rtp",
			Protocol:   "http+rtp",
			URL:        creds.SecretString(rawURL),
		},
		baseURL:     u,
		cameraID:    cameraID,
		apiKey:      apiKey,
		ctx:         ctx,
		cancel:      cancel,
		do:          tcp.Do,
		activeReady: make(chan struct{}),
		done:        make(chan struct{}),
	}

	camera, err := c.getCamera()
	if err != nil {
		cancel()
		return nil, err
	}

	log.Debug().
		Str("camera_id", cameraID).
		Bool("has_speaker", camera.FeatureFlags.HasSpeaker).
		Msg("[unifi] camera metadata")

	if camera.FeatureFlags.HasSpeaker {
		c.Medias = []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{
						Name:        core.CodecOpus,
						ClockRate:   48000,
						Channels:    2,
						PayloadType: 111,
					},
					{
						Name:        core.CodecPCMU,
						ClockRate:   8000,
						PayloadType: 0,
					},
					{
						Name:        core.CodecPCMA,
						ClockRate:   8000,
						PayloadType: 8,
					},
				},
			},
		}
	}

	return c, nil
}

func RegisterTalkbackSecrets(rawURL string) {
	innerURL := strings.TrimPrefix(rawURL, SchemeTalkback+":")
	if innerURL == rawURL {
		return
	}

	u, err := url.Parse(innerURL)
	if err != nil {
		return
	}

	creds.AddSecret(u.Query().Get("api_key"))
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	log.Debug().
		Str("camera_id", c.cameraID).
		Str("codec", track.Codec.String()).
		Msg("[unifi] add microphone track")

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		output, err := c.ensureActive(track.Codec)
		if err != nil {
			return
		}

		b, err := packet.Marshal()
		if err != nil {
			c.finish(err)
			return
		}

		_ = output.rtp.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
		if n, err := output.rtp.Write(b); err == nil {
			c.Send += n
		} else {
			log.Debug().Err(err).Msg("[unifi] write microphone RTP")
			c.finish(err)
		}
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)

	return nil
}

func (c *Client) Start() error {
	<-c.done

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Client) Stop() error {
	log.Debug().Str("camera_id", c.cameraID).Msg("[unifi] stop talkback")

	c.cancel()

	err := c.Connection.Stop()

	c.mu.Lock()
	c.stopped = true
	output := c.active
	c.active = nil
	c.mu.Unlock()

	if output != nil {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}

	c.finish(nil)
	return err
}

func (c *Client) ensureActive(inputCodec *core.Codec) (*talkbackOutput, error) {
	c.activeOnce.Do(func() {
		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()

		var output *talkbackOutput
		var err error
		if stopped {
			err = errStopped
		} else {
			log.Debug().
				Str("camera_id", c.cameraID).
				Str("codec", inputCodec.String()).
				Msg("[unifi] open talkback session")
			output, err = c.openTalkback(inputCodec)
		}

		c.mu.Lock()
		stopped = c.stopped
		if err == nil && stopped {
			err = errStopped
		}
		if err == nil {
			c.active = output
		}
		c.mu.Unlock()

		if err != nil {
			if output != nil {
				_ = output.Close()
			}
			if !stopped && !errors.Is(err, errStopped) {
				log.Warn().Err(err).Msg("[unifi] open talkback session")
				c.finish(err)
			}
		}

		close(c.activeReady)
	})

	<-c.activeReady

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return nil, c.err
	}
	if c.stopped || c.active == nil {
		return nil, errStopped
	}

	return c.active, nil
}

func (c *Client) openTalkback(inputCodec *core.Codec) (*talkbackOutput, error) {
	session, err := c.createTalkbackSession()
	if err != nil {
		return nil, err
	}

	registerSecretURL(session.URL)

	log.Debug().
		Str("camera_id", c.cameraID).
		Str("codec", session.Codec).
		Int("sampling_rate", session.SamplingRate).
		Int("bits_per_sample", session.BitsPerSample).
		Str("url", creds.SecretString(session.URL)).
		Msg("[unifi] talkback session")

	if !strings.EqualFold(session.Codec, "opus") {
		return nil, fmt.Errorf("unifi: unsupported talkback codec: %s", session.Codec)
	}
	if session.SamplingRate == 0 {
		return nil, errors.New("unifi: talkback samplingRate required")
	}

	output, err := startFFmpegRTP(inputCodec, session)
	if err != nil {
		return nil, err
	}

	go c.waitOutput(output)

	return output, nil
}

func (c *Client) waitOutput(output *talkbackOutput) {
	err := output.cmd.Wait()
	log.Debug().Err(err).Str("camera_id", c.cameraID).Msg("[unifi] ffmpeg stopped")

	c.mu.Lock()
	stopped := c.stopped
	if c.active == output {
		c.active = nil
	}
	c.mu.Unlock()

	_ = output.Close()

	if !stopped {
		c.finish(err)
	}
}

func (c *Client) getCamera() (*publicCamera, error) {
	var camera publicCamera
	if err := c.requestJSON(http.MethodGet, c.cameraPath(), nil, &camera); err != nil {
		return nil, err
	}
	return &camera, nil
}

func (c *Client) createTalkbackSession() (*TalkbackSession, error) {
	var session TalkbackSession
	if err := c.requestJSON(http.MethodPost, c.cameraPath()+"/talkback-session", nil, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) requestJSON(method, path string, body io.Reader, out any) error {
	u := *c.baseURL
	u.Path = path
	u.RawQuery = ""

	ts := time.Now()
	log.Debug().
		Str("method", method).
		Str("path", path).
		Msg("[unifi] protect api request")

	req, err := http.NewRequestWithContext(c.ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)

	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer tcp.Close(res)

	b, err := io.ReadAll(res.Body)
	if err != nil {
		log.Debug().
			Err(err).
			Str("method", method).
			Str("path", path).
			Int("status", res.StatusCode).
			Stringer("duration", time.Since(ts)).
			Msg("[unifi] protect api response")
		return err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		logProtectResponse(method, path, res.StatusCode, time.Since(ts), b)
		return fmt.Errorf("unifi: %s %s: %s", method, path, res.Status)
	}

	if err = json.Unmarshal(b, out); err != nil {
		logProtectResponse(method, path, res.StatusCode, time.Since(ts), b)
		return err
	}

	if session, ok := out.(*TalkbackSession); ok {
		registerSecretURL(session.URL)
	}

	logProtectResponse(method, path, res.StatusCode, time.Since(ts), b)
	return nil
}

func (c *Client) cameraPath() string {
	return "/proxy/protect/integration/v1/cameras/" + url.PathEscape(c.cameraID)
}

func logProtectResponse(method, path string, status int, duration time.Duration, body []byte) {
	log.Debug().
		Str("method", method).
		Str("path", path).
		Int("status", status).
		Stringer("duration", duration).
		Msg("[unifi] protect api response")

	if log.Trace().Enabled() {
		log.Trace().
			Str("method", method).
			Str("path", path).
			Str("body", creds.SecretString(string(body))).
			Msg("[unifi] protect api response body")
	}
}

func registerSecretURL(value string) {
	if value == "" {
		return
	}

	creds.AddSecret(value)

	jsonEscaped := strings.ReplaceAll(value, `/`, `\/`)
	creds.AddSecret(jsonEscaped)

	htmlEscaped := strings.ReplaceAll(value, `&`, `\u0026`)
	creds.AddSecret(htmlEscaped)
	creds.AddSecret(strings.ReplaceAll(htmlEscaped, `/`, `\/`))
}

func (c *Client) finish(err error) {
	if err != nil {
		c.mu.Lock()
		if c.err == nil {
			c.err = err
		}
		c.mu.Unlock()
	}

	c.doneOnce.Do(func() {
		close(c.done)
	})
}
