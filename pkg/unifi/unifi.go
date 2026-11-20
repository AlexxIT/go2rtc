package unifi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
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

// Client - UniFi Protect talkback (audio backchannel) via the Protect integration API
type Client struct {
	core.Connection

	// Backchannel - starts the process that sends audio from codec to the talkback session
	Backchannel func(session *Session, codec *core.Codec) (core.Producer, error) `json:"-"`

	url      *url.URL
	cameraID string
	apiKey   string

	ctx    context.Context
	cancel context.CancelFunc

	talk chan struct{}
	once sync.Once

	mu     sync.Mutex
	media  *core.Media
	codec  *core.Codec
	sender *core.Sender   // the last microphone
	pipe   *core.Receiver // to the backchannel
	prod   core.Producer
}

// clients - secure and insecure, without redirects, so X-API-KEY never goes to another host
var clients = [2]*http.Client{newClient(false), newClient(true)}

func newClient(insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// timeout - for each Protect API request
var timeout = core.ConnDialTimeout

// retryDelay - minimum time of one talkback attempt
var retryDelay = 5 * time.Second

type Session struct {
	URL          string `json:"url"`
	Codec        string `json:"codec"`
	SamplingRate int    `json:"samplingRate"`
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(strings.TrimPrefix(rawURL, "unifi:"))
	if err != nil {
		return nil, err
	}

	query := u.Query()
	cameraID, apiKey := query.Get("camera_id"), query.Get("api_key")
	if cameraID == "" || apiKey == "" {
		return nil, errors.New("unifi: camera_id and api_key required")
	}

	creds.AddSecret(apiKey)

	u.RawQuery = ""
	u.Fragment = ""

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "unifi",
			Protocol:   "http",
			URL:        rawURL,
		},
		url:      u,
		cameraID: cameraID,
		apiKey:   apiKey,
		ctx:      ctx,
		cancel:   cancel,
		talk:     make(chan struct{}),
	}

	var camera struct {
		FeatureFlags struct {
			HasSpeaker bool `json:"hasSpeaker"`
		} `json:"featureFlags"`
	}
	if err = c.request("GET", "", &camera); err != nil {
		cancel()
		return nil, err
	}

	if camera.FeatureFlags.HasSpeaker {
		c.Medias = []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecPCMA, ClockRate: 8000},
					{Name: core.CodecPCMU, ClockRate: 8000},
				},
			},
		}
	}

	return c, nil
}

func (c *Client) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

// AddTrack - the talkback session is opened only after the first microphone packet,
// so normal viewing does not activate the camera speaker. The last microphone wins.
func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	c.mu.Lock()
	if c.codec != nil && c.codec.Name != track.Codec.Name {
		c.mu.Unlock()
		return errors.New("unifi: microphone codec changed: " + track.Codec.Name)
	}

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.once.Do(func() { close(c.talk) })

		c.mu.Lock()
		pipe := c.pipe
		c.mu.Unlock()

		if pipe != nil {
			pipe.WriteRTP(packet)
		}
	}
	sender.HandleRTP(track)

	prev := c.sender
	c.media, c.codec, c.sender = media, track.Codec, sender
	c.Senders = []*core.Sender{sender}
	c.mu.Unlock()

	if prev != nil {
		prev.Close() // detach from the previous microphone
	}
	return nil
}

func (c *Client) Start() error {
	select {
	case <-c.talk:
	case <-c.ctx.Done():
		return nil
	}

	// streams reconnects at once after Start, so a failing talkback would POST at microphone packet rate
	defer func(ts time.Time) {
		select {
		case <-time.After(retryDelay - time.Since(ts)):
		case <-c.ctx.Done():
		}
	}(time.Now())

	var session Session
	if err := c.request("POST", "/talkback-session", &session); err != nil {
		return err
	}
	if err := session.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	media, codec := c.media, c.codec
	c.mu.Unlock()

	prod, err := c.Backchannel(&session, codec)
	if err != nil {
		return err
	}

	cons, ok := prod.(core.Consumer)
	if !ok {
		_ = prod.Stop()
		return errors.New("unifi: backchannel is not a consumer")
	}

	pipe := core.NewReceiver(media, codec)
	if err = cons.AddTrack(media, codec, pipe); err != nil {
		_ = prod.Stop()
		return err
	}

	c.mu.Lock()
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return prod.Stop()
	}
	c.prod, c.pipe = prod, pipe
	c.mu.Unlock()

	return prod.Start()
}

func (c *Client) Stop() error {
	c.cancel()

	c.mu.Lock()
	prod := c.prod
	c.mu.Unlock()

	if prod != nil {
		_ = prod.Stop()
	}

	return c.Connection.Stop()
}

func (c *Client) request(method, path string, v any) error {
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	u := *c.url
	u.Path = "/proxy/protect/integration/v1/cameras/" + url.PathEscape(c.cameraID) + path

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)

	// same rule as tcp.Do: self-signed certificate is OK for an IP address
	client := clients[0]
	switch {
	case u.Scheme == "httpx":
		req.URL.Scheme = "https"
		client = clients[1]
	case u.Scheme == "https" && tcp.IsIP(u.Hostname()):
		client = clients[1]
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return errors.New("unifi: " + method + " " + u.Path + ": " + res.Status)
	}

	return json.NewDecoder(res.Body).Decode(v)
}

func (s *Session) validate() error {
	if s.URL == "" {
		return errors.New("unifi: talkback url required")
	}
	if s.SamplingRate == 0 {
		return errors.New("unifi: talkback samplingRate required")
	}
	return nil
}
