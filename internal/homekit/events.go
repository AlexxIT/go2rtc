package homekit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
)

// HAP service/characteristic types for doorbell
const (
	TypeDoorbellService           = "121" // Doorbell service
	TypeProgrammableSwitchEvent   = "73"  // Programmable Switch Event characteristic
	TypeStatelessProgrammableSwitch = "89" // Stateless Programmable Switch service (fallback)
)

// Programmable Switch Event values
const (
	SwitchEventSinglePress = 0
	SwitchEventDoublePress = 1
	SwitchEventLongPress   = 2
)

// DoorbellEvent is sent to webhook and SSE listeners when the doorbell is pressed.
type DoorbellEvent struct {
	Stream    string `json:"stream"`
	Event     string `json:"event"`
	Value     int    `json:"value"`
	Timestamp string `json:"timestamp"`
}

type eventConfig struct {
	Stream  string `yaml:"stream"`
	Webhook string `yaml:"webhook"`
}

const maxWebhookResponseBody = 1 << 20 // 1 MiB

var (
	webhookHTTPClient = &http.Client{
		Timeout: 10 * time.Second,
		// Redirects can silently move a webhook to another host.
		// Keep the configured destination explicit.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	sseListenersMu sync.Mutex
	sseListeners   []chan DoorbellEvent
)

func addSSEListener(ch chan DoorbellEvent) {
	sseListenersMu.Lock()
	sseListeners = append(sseListeners, ch)
	sseListenersMu.Unlock()
}

func removeSSEListener(ch chan DoorbellEvent) {
	sseListenersMu.Lock()
	for i, l := range sseListeners {
		if l == ch {
			sseListeners = append(sseListeners[:i], sseListeners[i+1:]...)
			break
		}
	}
	sseListenersMu.Unlock()
}

func notifySSEListeners(ev DoorbellEvent) {
	sseListenersMu.Lock()
	for _, ch := range sseListeners {
		select {
		case ch <- ev:
		default: // don't block if listener is slow
		}
	}
	sseListenersMu.Unlock()
}

func initEvents() {
	var cfg struct {
		Mod map[string]eventConfig `yaml:"events"`
	}
	app.LoadConfig(&cfg)

	if cfg.Mod == nil {
		return
	}

	for name, conf := range cfg.Mod {
		streamName := conf.Stream
		if streamName == "" {
			streamName = name
		}
		go eventLoop(name, streamName, conf.Webhook)
	}
}

func eventLoop(name, streamName, webhookURL string) {
	const reconnectDelay = 10 * time.Second

	for {
		err := runEventListener(name, streamName, webhookURL)
		if err != nil {
			log.Error().Err(err).Msgf("[events] %s listener failed, reconnecting in %s", name, reconnectDelay)
		} else {
			log.Warn().Msgf("[events] %s listener disconnected, reconnecting in %s", name, reconnectDelay)
		}
		time.Sleep(reconnectDelay)
	}
}

func runEventListener(name, streamName, webhookURL string) error {
	// Get the homekit URL from the stream
	stream := streams.Get(streamName)
	if stream == nil {
		return fmt.Errorf("stream %q not found", streamName)
	}

	rawURL := findHomeKitURL(stream.Sources())
	if rawURL == "" {
		return fmt.Errorf("stream %q has no homekit source", streamName)
	}

	log.Info().Msgf("[events] %s: connecting to %s", name, streamName)

	client, err := hap.Dial(rawURL)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer client.Close()

	// Find the Programmable Switch Event characteristic
	acc, err := client.GetFirstAccessory()
	if err != nil {
		return fmt.Errorf("get accessories: %w", err)
	}

	switchEventIID := findSwitchEventIID(acc)
	if switchEventIID == 0 {
		return fmt.Errorf("no Programmable Switch Event characteristic found on %q", streamName)
	}

	log.Info().Msgf("[events] %s: found switch event characteristic IID=%d", name, switchEventIID)

	// Channel to signal when connection drops
	done := make(chan error, 1)

	// Set up event handler before starting the events reader
	client.OnEvent = func(res *http.Response) {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			log.Error().Err(err).Msgf("[events] %s: read event body", name)
			return
		}

		var chars hap.JSONCharacters
		if err := json.Unmarshal(body, &chars); err != nil {
			log.Error().Err(err).Msgf("[events] %s: unmarshal event", name)
			return
		}

		for _, char := range chars.Value {
			if char.IID != switchEventIID {
				continue
			}

			value := 0
			if v, ok := char.Value.(float64); ok {
				value = int(v)
			}

			eventName := "single_press"
			switch value {
			case SwitchEventDoublePress:
				eventName = "double_press"
			case SwitchEventLongPress:
				eventName = "long_press"
			}

			log.Info().Msgf("[events] %s: doorbell %s", name, eventName)

			ev := DoorbellEvent{
				Stream:    name,
				Event:     eventName,
				Value:     value,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}

			notifySSEListeners(ev)

			if webhookURL != "" {
				go fireWebhook(webhookURL, ev)
			}
		}
	}

	// Start the events reader goroutine — this demuxes events from
	// regular HTTP responses on the encrypted HAP connection.
	client.StartEventsReader()

	// Subscribe to the Programmable Switch Event characteristic
	if err := client.SubscribeEvent(switchEventIID); err != nil {
		return fmt.Errorf("subscribe event: %w", err)
	}

	log.Info().Msgf("[events] %s: subscribed to doorbell events", name)

	// Keep the connection alive by reading until it drops.
	// The eventsReader goroutine handles all incoming data; when the
	// connection closes, the res channel will be closed and we'll get
	// an error or zero-value read here.
	//
	// We use a keepalive ping (reading a characteristic) to detect
	// dead connections faster than TCP timeouts.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := client.GetCharacters(fmt.Sprintf("1.%d", switchEventIID)); err != nil {
				done <- fmt.Errorf("keepalive failed: %w", err)
				return
			}
		}
	}()

	return <-done
}

// findSwitchEventIID searches the accessory for a Programmable Switch Event
// characteristic. It first looks in the Doorbell service (type "121"), then
// falls back to Stateless Programmable Switch (type "89").
func findSwitchEventIID(acc *hap.Accessory) uint64 {
	// Try Doorbell service first
	for _, svc := range acc.Services {
		if svc.Type == TypeDoorbellService {
			for _, char := range svc.Characters {
				if char.Type == TypeProgrammableSwitchEvent {
					return char.IID
				}
			}
		}
	}

	// Fallback: Stateless Programmable Switch service
	for _, svc := range acc.Services {
		if svc.Type == TypeStatelessProgrammableSwitch {
			for _, char := range svc.Characters {
				if char.Type == TypeProgrammableSwitchEvent {
					return char.IID
				}
			}
		}
	}

	// Last resort: any service with the characteristic
	for _, svc := range acc.Services {
		for _, char := range svc.Characters {
			if char.Type == TypeProgrammableSwitchEvent {
				return char.IID
			}
		}
	}

	return 0
}

func fireWebhook(url string, ev DoorbellEvent) {
	webhookURL, err := validateWebhookURL(url)
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("[events] invalid webhook URL")
		return
	}

	body, err := json.Marshal(ev)
	if err != nil {
		log.Error().Err(err).Msg("[events] marshal webhook body")
		return
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL.String(), bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("[events] build webhook request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookHTTPClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("[events] webhook POST")
		return
	}
	defer resp.Body.Close()

	if _, err = io.Copy(io.Discard, io.LimitReader(resp.Body, maxWebhookResponseBody)); err != nil {
		log.Debug().Err(err).Str("url", url).Msg("[events] drain webhook response")
	}

	switch {
	case resp.StatusCode >= 400:
		log.Warn().Str("url", url).Int("status", resp.StatusCode).Msg("[events] webhook returned error status")
	case resp.StatusCode >= 300:
		log.Warn().Str("url", url).Int("status", resp.StatusCode).Msg("[events] webhook returned redirect status")
	default:
		log.Trace().Str("url", url).Int("status", resp.StatusCode).Msg("[events] webhook returned status")
	}
}

func validateWebhookURL(rawURL string) (*neturl.URL, error) {
	webhookURL, err := neturl.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse webhook URL: %w", err)
	}
	if webhookURL.Scheme != "http" && webhookURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported webhook scheme %q", webhookURL.Scheme)
	}
	if webhookURL.Host == "" {
		return nil, fmt.Errorf("webhook URL missing host")
	}
	return webhookURL, nil
}
