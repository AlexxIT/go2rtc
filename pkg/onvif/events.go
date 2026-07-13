package onvif

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// EventSubscription holds state for an ONVIF PullPoint event subscription.
type EventSubscription struct {
	client *Client
	// address is the PullPoint subscription manager URL (from CreatePullPointSubscription response).
	address string
}

// CreatePullPointSubscription creates an ONVIF PullPoint subscription on the camera's event service.
// The timeout specifies how long the subscription stays alive before needing renewal.
func (c *Client) CreatePullPointSubscription(timeout time.Duration) (*EventSubscription, error) {
	if c.eventURL == "" {
		return nil, errors.New("onvif: event service not available")
	}

	secs := int(timeout.Seconds())
	if secs < 10 {
		secs = 10
	}

	log.Debug().Str("event_url", c.eventURL).Int("timeout_secs", secs).
		Msg("[onvif] creating pull point subscription")

	body := fmt.Sprintf(`<tev:CreatePullPointSubscription>`+
		`<tev:InitialTerminationTime>PT%dS</tev:InitialTerminationTime>`+
		`</tev:CreatePullPointSubscription>`, secs)

	b, err := c.EventRequest(c.eventURL, body)
	if err != nil {
		return nil, fmt.Errorf("onvif: create pull point: %w", err)
	}

	log.Trace().Str("response", string(b)).Msg("[onvif] create pull point response")

	// Extract subscription reference address from response.
	// Response contains: <wsnt:SubscriptionReference><wsa:Address>http://...</wsa:Address>
	addr := FindTagValue(b, "Address")
	if addr == "" {
		return nil, errors.New("onvif: no subscription address in response")
	}

	log.Debug().Str("raw_address", addr).Msg("[onvif] subscription address from camera")

	// Some cameras return relative paths or localhost — fix using camera's host.
	resolved := c.resolveEventAddress(addr)

	log.Debug().Str("resolved_address", resolved).Msg("[onvif] subscription address resolved")

	return &EventSubscription{
		client:  c,
		address: resolved,
	}, nil
}

// PullMessages polls the camera for events. This is a long-poll: it blocks
// up to the specified timeout waiting for events. Returns raw XML response.
func (s *EventSubscription) PullMessages(timeout time.Duration, limit int) ([]byte, error) {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	if limit < 1 {
		limit = 1
	}

	body := fmt.Sprintf(`<tev:PullMessages>`+
		`<tev:Timeout>PT%dS</tev:Timeout>`+
		`<tev:MessageLimit>%d</tev:MessageLimit>`+
		`</tev:PullMessages>`, secs, limit)

	return s.client.EventRequest(s.address, body)
}

// Renew extends the subscription lifetime by the specified duration.
func (s *EventSubscription) Renew(timeout time.Duration) error {
	secs := int(timeout.Seconds())

	log.Trace().Str("address", s.address).Int("timeout_secs", secs).
		Msg("[onvif] renewing subscription")

	body := fmt.Sprintf(`<wsnt:Renew>`+
		`<wsnt:TerminationTime>PT%dS</wsnt:TerminationTime>`+
		`</wsnt:Renew>`, secs)

	_, err := s.client.EventRequest(s.address, body)
	return err
}

// Unsubscribe terminates the subscription on the camera (best-effort).
func (s *EventSubscription) Unsubscribe() error {
	log.Trace().Str("address", s.address).Msg("[onvif] unsubscribing")
	_, err := s.client.EventRequest(s.address, `<wsnt:Unsubscribe/>`)
	return err
}

// EventRequest sends a SOAP request with event-specific namespaces.
func (c *Client) EventRequest(reqURL, body string) ([]byte, error) {
	if reqURL == "" {
		return nil, errors.New("onvif: unsupported service")
	}

	e := NewEventEnvelopeWithUser(c.url.User)
	e.Append(body)

	log.Trace().Str("url", reqURL).Msg("[onvif] event request sending")

	// Use a longer timeout for PullMessages (long-poll).
	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Post(reqURL, `application/soap+xml;charset=utf-8`, bytes.NewReader(e.Bytes()))
	if err != nil {
		log.Trace().Err(err).Str("url", reqURL).Msg("[onvif] event request failed")
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		log.Debug().Str("url", reqURL).Int("status", res.StatusCode).
			Msg("[onvif] event request non-200 response")
		return nil, errors.New("onvif: event request failed " + res.Status)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Trace().Str("url", reqURL).Int("bytes", len(b)).
		Msg("[onvif] event request response received")

	return b, nil
}

// resolveEventAddress fixes subscription addresses returned by the camera.
// The camera may return its internal IP, localhost, or a relative path.
// We always use the host from the original client URL since we know it's reachable.
func (c *Client) resolveEventAddress(addr string) string {
	u, err := url.Parse(addr)
	if err != nil {
		return addr
	}

	// Always use the host we connected to (handles Docker, NAT, port mapping, etc.).
	u.Host = c.url.Host

	if u.Scheme == "" {
		u.Scheme = "http"
	}

	return u.String()
}

// ParseMotionEvents extracts motion state from a PullMessages response.
// Returns (motionDetected, found). If no motion-related notification is present, found=false.
//
// Recognizes common ONVIF motion event topics:
//   - tns1:RuleEngine/CellMotionDetector/Motion (IsMotion property)
//   - tns1:VideoSource/MotionAlarm (State property)
//   - tns1:RuleEngine/MotionRegionDetector/Motion
func ParseMotionEvents(b []byte) (motion bool, found bool) {
	s := string(b)

	// Find notification messages containing motion-related topics.
	reTopic := regexp.MustCompile(`(?s)<[^>]*Topic[^>]*>([^<]*)</`)
	reValue := regexp.MustCompile(`SimpleItem[^>]+Name="(IsMotion|State)"[^>]+Value="(\w+)"`)

	topics := reTopic.FindAllStringSubmatch(s, -1)
	if len(topics) == 0 {
		log.Trace().Msg("[onvif] parse: no topics found in response")
		return false, false
	}

	log.Trace().Int("topic_count", len(topics)).Msg("[onvif] parse: topics found")
	for i, t := range topics {
		if len(t) >= 2 {
			log.Trace().Int("idx", i).Str("topic", t[1]).Msg("[onvif] parse: topic")
		}
	}

	// Split response into individual NotificationMessage blocks.
	messages := splitNotificationMessages(s)

	log.Trace().Int("message_count", len(messages)).Msg("[onvif] parse: notification messages")

	for _, msg := range messages {
		// Check if this message's topic is motion-related.
		topicMatch := reTopic.FindStringSubmatch(msg)
		if len(topicMatch) < 2 {
			log.Trace().Msg("[onvif] parse: message has no topic, skipping")
			continue
		}
		topic := topicMatch[1]

		if !isMotionTopic(topic) {
			log.Trace().Str("topic", topic).Msg("[onvif] parse: non-motion topic, skipping")
			continue
		}

		log.Trace().Str("topic", topic).Msg("[onvif] parse: motion topic found")

		// Extract the motion value from this message.
		valueMatch := reValue.FindStringSubmatch(msg)
		if len(valueMatch) < 3 {
			log.Trace().Str("topic", topic).Msg("[onvif] parse: no IsMotion/State value in message")
			continue
		}

		val := strings.ToLower(valueMatch[2])
		motion = val == "true" || val == "1"
		found = true

		log.Trace().Str("topic", topic).Str("name", valueMatch[1]).
			Str("value", valueMatch[2]).Bool("motion", motion).
			Msg("[onvif] parse: motion value extracted")
		// Use the last motion event if multiple are present.
	}

	return motion, found
}

// isMotionTopic checks if a topic string relates to motion detection.
func isMotionTopic(topic string) bool {
	topic = strings.ToLower(topic)
	return strings.Contains(topic, "motiondetector") ||
		strings.Contains(topic, "motionalarm") ||
		strings.Contains(topic, "motionregiondetector") ||
		strings.Contains(topic, "cellmotiondetector")
}

// splitNotificationMessages splits the XML response into individual notification message blocks.
func splitNotificationMessages(s string) []string {
	re := regexp.MustCompile(`(?s)<[^>]*NotificationMessage[^>]*>.*?</[^>]*NotificationMessage>`)
	return re.FindAllString(s, -1)
}
