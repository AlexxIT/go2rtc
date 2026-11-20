package onvif

import (
	"context"
	"encoding/xml"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// ONVIF Events service, PullPoint only (WS-BaseNotification push and
// the pausable subscription manager are not supported). Supports the motion topics:
//   - tns1:RuleEngine/CellMotionDetector/Motion (IsMotion)
//   - tns1:VideoSource/MotionAlarm (State)

const (
	EventsGetEventProperties          = "GetEventProperties"
	EventsCreatePullPointSubscription = "CreatePullPointSubscription"
	EventsPullMessages                = "PullMessages"
	EventsRenew                       = "Renew"
	EventsUnsubscribe                 = "Unsubscribe"
	EventsSetSynchronizationPoint     = "SetSynchronizationPoint"
)

const (
	PathEvents       = "/onvif/events_service"
	PathSubscription = "/onvif/subscription/"

	eventsNS = ` xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wsa5="http://www.w3.org/2005/08/addressing" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1" xmlns:tns1="http://www.onvif.org/ver10/topics"`

	topicDialect = "http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet"

	subscriptionTTLMin     = 10 * time.Second
	subscriptionTTLDefault = 60 * time.Second
	subscriptionTTLMax     = time.Hour
	subscriptionsMax       = 16
	queueMax               = 64
	pullTimeoutMax         = time.Minute
)

type eventMessage struct {
	token     string
	motion    bool
	operation string // Initialized or Changed
	time      time.Time
}

type subscription struct {
	id      string
	expires time.Time
	queue   []eventMessage
	wake    chan struct{} // closed and replaced on a new message
}

var ErrUnknownSubscription = errors.New("onvif: unknown subscription")

// Events is a thread safe storage for motion states and PullPoint subscriptions.
type Events struct {
	mu     sync.Mutex
	tokens []string        // source tokens in the order of creation
	states map[string]bool // source token => motion
	subs   map[string]*subscription
}

func NewEvents() *Events {
	return &Events{states: map[string]bool{}, subs: map[string]*subscription{}}
}

// AddSource registers a source (go2rtc stream name), which can report motion.
func (e *Events) AddSource(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.states[token]; !ok {
		e.tokens = append(e.tokens, token)
		e.states[token] = false
	}
}

func (e *Events) HasSources() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.tokens) > 0
}

// SetMotion changes the state of the source and notifies all subscribers.
// An unknown source is registered on the first call.
func (e *Events) SetMotion(token string, motion bool) {
	now := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.gc(now)

	state, ok := e.states[token]
	if !ok {
		e.tokens = append(e.tokens, token)
	} else if state == motion {
		return
	}
	e.states[token] = motion

	msg := eventMessage{token: token, motion: motion, operation: "Changed", time: now}
	for _, sub := range e.subs {
		sub.push(msg)
	}
}

func (s *subscription) push(msg eventMessage) {
	if len(s.queue) >= queueMax {
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, msg)
	s.notify()
}

// notify wakes up all pending Pull calls of the subscription
func (s *subscription) notify() {
	close(s.wake)
	s.wake = make(chan struct{})
}

// remove deletes the subscription and wakes up its pending Pull calls,
// so they return an unknown subscription instead of waiting for the timeout
func (e *Events) remove(id string) {
	if sub := e.subs[id]; sub != nil {
		delete(e.subs, id)
		close(sub.wake)
	}
}

func (e *Events) gc(now time.Time) {
	for id, sub := range e.subs {
		if now.After(sub.expires) {
			e.remove(id)
		}
	}
}

func clampTTL(ttl time.Duration) time.Duration {
	switch {
	case ttl <= 0:
		return subscriptionTTLDefault
	case ttl < subscriptionTTLMin:
		return subscriptionTTLMin
	case ttl > subscriptionTTLMax:
		return subscriptionTTLMax
	}
	return ttl
}

// Subscribe creates a new subscription with the current states in the queue.
func (e *Events) Subscribe(ttl time.Duration) (id string, expires time.Time, ok bool) {
	now := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.gc(now)
	if len(e.subs) >= subscriptionsMax {
		return "", time.Time{}, false
	}

	sub := &subscription{
		id:      core.RandString(16, 16),
		expires: now.Add(clampTTL(ttl)),
		wake:    make(chan struct{}),
	}
	sub.sync(e, now)
	e.subs[sub.id] = sub

	return sub.id, sub.expires, true
}

func (s *subscription) sync(e *Events, now time.Time) {
	for _, token := range e.tokens {
		s.queue = append(s.queue, eventMessage{
			token: token, motion: e.states[token], operation: "Initialized", time: now,
		})
	}
}

// Sync puts the current states in the queue (SetSynchronizationPoint).
func (e *Events) Sync(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	sub := e.subs[id]
	if sub == nil {
		return false
	}
	sub.sync(e, time.Now())
	sub.notify()
	return true
}

func (e *Events) Renew(id string, ttl time.Duration) (expires time.Time, ok bool) {
	now := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.gc(now)
	sub := e.subs[id]
	if sub == nil {
		return time.Time{}, false
	}
	sub.expires = now.Add(clampTTL(ttl))
	return sub.expires, true
}

func (e *Events) Unsubscribe(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.subs[id] == nil {
		return false
	}
	e.remove(id)
	return true
}

// Pull waits for messages up to the timeout. The response is ready to send.
// Returns ErrUnknownSubscription for an unknown (or expired) subscription and
// the context error if the context is done (ex. the client disconnected).
func (e *Events) Pull(ctx context.Context, id string, timeout time.Duration, limit int) ([]byte, error) {
	if timeout > pullTimeoutMax {
		timeout = pullTimeoutMax
	}
	if limit <= 0 {
		limit = queueMax
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		now := time.Now()

		e.mu.Lock()
		e.gc(now)
		sub := e.subs[id]
		if sub == nil {
			e.mu.Unlock()
			return nil, ErrUnknownSubscription
		}

		// a pull keeps the subscription alive for the time of the request
		if keep := now.Add(timeout + subscriptionTTLMin); sub.expires.Before(keep) {
			sub.expires = keep
		}

		if n := len(sub.queue); n > 0 {
			if n > limit {
				n = limit
			}
			msgs := sub.queue[:n:n]
			sub.queue = sub.queue[n:]
			expires := sub.expires
			e.mu.Unlock()
			return pullMessagesResponse(now, expires, msgs), nil
		}

		wake := sub.wake
		expires := sub.expires
		e.mu.Unlock()

		select {
		case <-wake:
		case <-deadline.C:
			return pullMessagesResponse(time.Now(), expires, nil), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// escape a string for XML text and attribute values (stream names come from the config)
func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func pullMessagesResponse(now, expires time.Time, msgs []eventMessage) []byte {
	e := NewEnvelope()
	e.Appendf(`<tev:PullMessagesResponse%s>
	<tev:CurrentTime>%s</tev:CurrentTime>
	<tev:TerminationTime>%s</tev:TerminationTime>`, eventsNS, formatTime(now), formatTime(expires))
	for _, msg := range msgs {
		ts := formatTime(msg.time)
		token := escape(msg.token)
		e.Appendf(`
	<wsnt:NotificationMessage>
		<wsnt:Topic Dialect="%s">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
		<wsnt:Message>
			<tt:Message UtcTime="%s" PropertyOperation="%s">
				<tt:Source>
					<tt:SimpleItem Name="VideoSourceConfigurationToken" Value="%s"/>
					<tt:SimpleItem Name="VideoAnalyticsConfigurationToken" Value="%s"/>
					<tt:SimpleItem Name="Rule" Value="MotionDetectorRule"/>
				</tt:Source>
				<tt:Data><tt:SimpleItem Name="IsMotion" Value="%t"/></tt:Data>
			</tt:Message>
		</wsnt:Message>
	</wsnt:NotificationMessage>
	<wsnt:NotificationMessage>
		<wsnt:Topic Dialect="%s">tns1:VideoSource/MotionAlarm</wsnt:Topic>
		<wsnt:Message>
			<tt:Message UtcTime="%s" PropertyOperation="%s">
				<tt:Source><tt:SimpleItem Name="Source" Value="%s"/></tt:Source>
				<tt:Data><tt:SimpleItem Name="State" Value="%t"/></tt:Data>
			</tt:Message>
		</wsnt:Message>
	</wsnt:NotificationMessage>`,
			topicDialect, ts, msg.operation, token, token, msg.motion,
			topicDialect, ts, msg.operation, token, msg.motion)
	}
	e.Append(`
</tev:PullMessagesResponse>`)
	return e.Bytes()
}

func CreatePullPointSubscriptionResponse(host, id string, expires time.Time) []byte {
	e := NewEnvelope()
	e.Appendf(`<tev:CreatePullPointSubscriptionResponse%s>
	<tev:SubscriptionReference><wsa5:Address>http://%s%s%s</wsa5:Address></tev:SubscriptionReference>
	<wsnt:CurrentTime>%s</wsnt:CurrentTime>
	<wsnt:TerminationTime>%s</wsnt:TerminationTime>
</tev:CreatePullPointSubscriptionResponse>`,
		eventsNS, escape(host), PathSubscription, escape(id), formatTime(time.Now()), formatTime(expires))
	return e.Bytes()
}

func RenewResponse(expires time.Time) []byte {
	e := NewEnvelope()
	e.Appendf(`<wsnt:RenewResponse%s>
	<wsnt:TerminationTime>%s</wsnt:TerminationTime>
	<wsnt:CurrentTime>%s</wsnt:CurrentTime>
</wsnt:RenewResponse>`, eventsNS, formatTime(expires), formatTime(time.Now()))
	return e.Bytes()
}

func UnsubscribeResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<wsnt:UnsubscribeResponse%s />`, eventsNS)
	return e.Bytes()
}

func SetSynchronizationPointResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<tev:SetSynchronizationPointResponse%s />`, eventsNS)
	return e.Bytes()
}

func EventsServiceCapabilitiesResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<tev:GetServiceCapabilitiesResponse%s>
	<tev:Capabilities WSSubscriptionPolicySupport="false" WSPullPointSupport="true" WSPausableSubscriptionManagerInterfaceSupport="false" MaxNotificationProducers="0" MaxPullPoints="%d" PersistentNotificationStorage="false" />
</tev:GetServiceCapabilitiesResponse>`, eventsNS, subscriptionsMax)
	return e.Bytes()
}

func GetEventPropertiesResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<tev:GetEventPropertiesResponse%s>
	<tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
	<wsnt:FixedTopicSet>true</wsnt:FixedTopicSet>
	<wstop:TopicSet>
		<tns1:RuleEngine>
			<CellMotionDetector>
				<Motion wstop:topic="true">
					<tt:MessageDescription IsProperty="true">
						<tt:Source>
							<tt:SimpleItemDescription Name="VideoSourceConfigurationToken" Type="tt:ReferenceToken"/>
							<tt:SimpleItemDescription Name="VideoAnalyticsConfigurationToken" Type="tt:ReferenceToken"/>
							<tt:SimpleItemDescription Name="Rule" Type="xs:string"/>
						</tt:Source>
						<tt:Data><tt:SimpleItemDescription Name="IsMotion" Type="xs:boolean"/></tt:Data>
					</tt:MessageDescription>
				</Motion>
			</CellMotionDetector>
		</tns1:RuleEngine>
		<tns1:VideoSource>
			<MotionAlarm wstop:topic="true">
				<tt:MessageDescription IsProperty="true">
					<tt:Source><tt:SimpleItemDescription Name="Source" Type="tt:ReferenceToken"/></tt:Source>
					<tt:Data><tt:SimpleItemDescription Name="State" Type="xs:boolean"/></tt:Data>
				</tt:MessageDescription>
			</MotionAlarm>
		</tns1:VideoSource>
	</wstop:TopicSet>
	<wsnt:TopicExpressionDialect>%s</wsnt:TopicExpressionDialect>
	<wsnt:TopicExpressionDialect>http://docs.oasis-open.org/wsn/t-1/TopicExpression/Concrete</wsnt:TopicExpressionDialect>
	<tev:MessageContentFilterDialect>http://www.onvif.org/ver10/tev/messageContentFilter/ItemFilter</tev:MessageContentFilterDialect>
	<tev:MessageContentSchemaLocation>http://www.onvif.org/onvif/ver10/schema/onvif.xsd</tev:MessageContentSchemaLocation>
</tev:GetEventPropertiesResponse>`, eventsNS+` xmlns:xs="http://www.w3.org/2001/XMLSchema"`, topicDialect)
	return e.Bytes()
}

// ResourceUnknownResponse is a fault for an unknown subscription.
func ResourceUnknownResponse() []byte {
	e := NewEnvelope()
	e.Append(`<s:Fault>
	<s:Code>
		<s:Value>s:Sender</s:Value>
		<s:Subcode><s:Value xmlns:wsrf-rw="http://docs.oasis-open.org/wsrf/rw-2">wsrf-rw:ResourceUnknownFault</s:Value></s:Subcode>
	</s:Code>
	<s:Reason><s:Text xml:lang="en">Unknown subscription</s:Text></s:Reason>
</s:Fault>`)
	return e.Bytes()
}

// SubscriptionID from the request path: /onvif/subscription/{id}
func SubscriptionID(path string) string {
	if i := strings.Index(path, PathSubscription); i >= 0 {
		return path[i+len(PathSubscription):]
	}
	return ""
}

var reDuration = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseDuration supports the xs:duration (PT60S, PT1M, P1D...) and
// the xs:dateTime (absolute time). Returns zero for an unknown format.
func ParseDuration(s string, now time.Time) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if m := reDuration.FindStringSubmatch(s); m != nil {
		var d time.Duration
		if m[1] != "" {
			n, _ := strconv.Atoi(m[1])
			d += time.Duration(n) * 24 * time.Hour
		}
		if m[2] != "" {
			n, _ := strconv.Atoi(m[2])
			d += time.Duration(n) * time.Hour
		}
		if m[3] != "" {
			n, _ := strconv.Atoi(m[3])
			d += time.Duration(n) * time.Minute
		}
		if m[4] != "" {
			f, _ := strconv.ParseFloat(m[4], 64)
			d += time.Duration(f * float64(time.Second))
		}
		return d
	}

	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Sub(now)
		}
	}
	return 0
}
