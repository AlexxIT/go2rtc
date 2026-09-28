package nest

import (
	"errors"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
)

type WebRTCClient struct {
	conn      *webrtc.Conn
	api       *API
	stream    *Stream
	keepalive *session
}

type RTSPClient struct {
	conn      *rtsp.Conn
	api       *API
	stream    *Stream
	keepalive *session
}

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	cliendID := query.Get("client_id")
	cliendSecret := query.Get("client_secret")
	refreshToken := query.Get("refresh_token")
	projectID := query.Get("project_id")
	deviceID := query.Get("device_id")

	if cliendID == "" || cliendSecret == "" || refreshToken == "" || projectID == "" || deviceID == "" {
		return nil, errors.New("nest: wrong query")
	}

	maxRetries := 3
	retryDelay := time.Second * 30

	var nestAPI *API
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		nestAPI, err = NewAPI(cliendID, cliendSecret, refreshToken)
		if err == nil {
			break
		}
		lastErr = err
		if attempt < maxRetries-1 {
			time.Sleep(retryDelay)
			retryDelay *= 2 // exponential backoff
		}
	}

	if nestAPI == nil {
		return nil, lastErr
	}

	protocols := strings.Split(query.Get("protocols"), ",")
	if len(protocols) > 0 && protocols[0] == "RTSP" {
		return rtspConn(nestAPI, rawURL, projectID, deviceID)
	}

	// Default to WEB_RTC for backwards compataiility
	return rtcConn(nestAPI, rawURL, projectID, deviceID)
}

func (c *WebRTCClient) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *WebRTCClient) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *WebRTCClient) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	return c.conn.AddTrack(media, codec, track)
}

func (c *WebRTCClient) Start() error {
	c.keepalive = c.api.keepAlive(c.stream)
	return c.conn.Start()
}

func (c *WebRTCClient) Stop() error {
	c.keepalive.Stop()
	return c.conn.Stop()
}

func (c *WebRTCClient) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}

// Google answers a switched-off camera with 400 FAILED_PRECONDITION on every
// GenerateWebRtcStream, and each of those calls counts against the project's
// per-minute command quota. A dashboard that polls a few off cameras every
// ten seconds is enough to earn 429s for every camera. Remember a definitive
// answer per device and fail repeated dials instantly from memory, so the
// fallback source takes over without a round trip. The hold-off doubles
// every time Google repeats the verdict - a camera that has been off for an
// hour is asked again every five minutes, not every minute - and resets as
// soon as a dial succeeds.
const (
	definitiveErrorMinTTL = time.Minute
	definitiveErrorMaxTTL = 5 * time.Minute
)

type definitiveError struct {
	err   error
	ttl   time.Duration
	until time.Time
}

type definitiveCache struct {
	mu      sync.Mutex
	entries map[string]definitiveError
	now     func() time.Time
}

var definitive = &definitiveCache{entries: map[string]definitiveError{}, now: time.Now}

// recent returns the cached verdict for a device while its hold-off lasts.
func (c *definitiveCache) recent(deviceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d, ok := c.entries[deviceID]; ok && c.now().Before(d.until) {
		return d.err
	}
	return nil
}

// remember records a verdict and returns the hold-off it will be honoured
// for: the minimum on a first failure, double the previous one on a repeat.
func (c *definitiveCache) remember(deviceID string, err error) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := definitiveErrorMinTTL
	if d, ok := c.entries[deviceID]; ok {
		ttl = d.ttl * 2
		if ttl > definitiveErrorMaxTTL {
			ttl = definitiveErrorMaxTTL
		}
	}
	c.entries[deviceID] = definitiveError{err: err, ttl: ttl, until: c.now().Add(ttl)}
	return ttl
}

// forget clears a device's verdict once a dial succeeds, so the next
// failure starts the back-off from the minimum again.
func (c *definitiveCache) forget(deviceID string) {
	c.mu.Lock()
	delete(c.entries, deviceID)
	c.mu.Unlock()
}

// isDeviceAnswer reports whether an error is Google's verdict on the device
// (400 camera off, 404 unknown) rather than a quota or local throttling
// answer that says nothing about the camera.
func isDeviceAnswer(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code >= 400 && se.Code < 500 && se.Code != 429
}

func recentDefinitiveError(deviceID string) error { return definitive.recent(deviceID) }

// rejectedOfferRetryDelay is the pause before re-offering after Google
// rejects an offer's SDP.
const rejectedOfferRetryDelay = time.Second

func rtcConn(nestAPI *API, rawURL, projectID, deviceID string) (*WebRTCClient, error) {
	maxRetries := 3
	retryDelay := time.Second * 30
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		rtcAPI, err := webrtc.NewAPI()
		if err != nil {
			return nil, err
		}

		conf := pion.Configuration{}
		pc, err := rtcAPI.NewPeerConnection(conf)
		if err != nil {
			return nil, err
		}

		conn := webrtc.NewConn(pc)
		conn.FormatName = "nest/webrtc"
		conn.Mode = core.ModeActiveProducer
		conn.Protocol = "http"
		conn.URL = rawURL

		// https://developers.google.com/nest/device-access/traits/device/camera-live-stream#generatewebrtcstream-request-fields
		medias := []*core.Media{
			{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
			{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
			{Kind: "app"}, // important for Nest
		}

		// 3. Create offer with candidates
		offer, err := conn.CreateCompleteOffer(medias)
		if err != nil {
			return nil, err
		}

		// 4. Exchange SDP via Hass
		if err := recentDefinitiveError(deviceID); err != nil {
			return nil, err
		}

		answer, stream, err := nestAPI.ExchangeSDP(projectID, deviceID, offer)
		if err != nil {
			lastErr = err
			// a switched-off camera (400 FAILED_PRECONDITION) or an unknown
			// device (404) will not change within the 90s retry window; a
			// rejected offer (400 INVALID_ARGUMENT) is retried below
			if !retryable(err) {
				if isDeviceAnswer(err) {
					ttl := definitive.remember(deviceID, err)
					log.Printf("nest: %s: not asking Google again for %s", err, ttl)
				}
				return nil, err
			}
			if attempt < maxRetries-1 {
				if rejectedOffer(err) {
					// a fresh offer is usually accepted; waiting out the
					// server-error delay would outlast most consumers' timeouts
					log.Printf("nest: %s: rejected offer:\n%s", err, offer)
					time.Sleep(rejectedOfferRetryDelay)
					continue
				}
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}
			return nil, err
		}

		// 5. Set answer with remote medias
		if err = conn.SetAnswer(answer); err != nil {
			return nil, err
		}

		definitive.forget(deviceID)

		return &WebRTCClient{conn: conn, api: nestAPI, stream: stream}, nil
	}

	return nil, lastErr
}

func rtspConn(nestAPI *API, rawURL, projectID, deviceID string) (*RTSPClient, error) {
	rtspURL, stream, err := nestAPI.GenerateRtspStream(projectID, deviceID)
	if err != nil {
		return nil, err
	}

	rtspClient := rtsp.NewClient(rtspURL)
	if err := rtspClient.Dial(); err != nil {
		return nil, err
	}
	if err := rtspClient.Describe(); err != nil {
		return nil, err
	}

	return &RTSPClient{conn: rtspClient, api: nestAPI, stream: stream}, nil
}

func (c *RTSPClient) GetMedias() []*core.Media {
	result := c.conn.GetMedias()
	return result
}

func (c *RTSPClient) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *RTSPClient) Start() error {
	c.keepalive = c.api.keepAlive(c.stream)
	return c.conn.Start()
}

func (c *RTSPClient) Stop() error {
	c.keepalive.Stop()
	_ = c.api.StopRTSPStream(c.stream)
	return c.conn.Stop()
}

func (c *RTSPClient) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}
