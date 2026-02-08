package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
)

// Event represents a stream-related event
type Event struct {
	Type      string    `json:"type"`       // stream_added, stream_removed, stream_updated, producer_connected, etc.
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// EventBus manages event subscriptions and broadcasts
type EventBus struct {
	subscribers map[chan Event]struct{}
	mu          sync.RWMutex
}

var eventBus = &EventBus{
	subscribers: make(map[chan Event]struct{}),
}

// Subscribe registers a new subscriber to receive events
func (eb *EventBus) Subscribe() chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, 64)
	eb.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber
func (eb *EventBus) Unsubscribe(ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	delete(eb.subscribers, ch)
	close(ch)
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber
		}
	}
}

// StreamEventNotifier monitors stream changes and publishes events
func StreamEventNotifier(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	knownStreams := make(map[string][]string)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkStreamChanges(knownStreams)
		}
	}
}

func checkStreamChanges(knownStreams map[string][]string) {
	currentNames := streams.GetAllNames()

	// Check for new or updated streams
	for _, name := range currentNames {
		stream := streams.Get(name)
		if stream == nil {
			continue
		}

		currentSources := stream.Sources()
		previousSources, exists := knownStreams[name]

		if !exists {
			// New stream
			eventBus.Publish(Event{
				Type:      "stream_added",
				Timestamp: time.Now(),
				Data: map[string]any{
					"name":    name,
					"sources": currentSources,
				},
			})
		} else if !sourcesEqual(previousSources, currentSources) {
			// Updated stream
			eventBus.Publish(Event{
				Type:      "stream_updated",
				Timestamp: time.Now(),
				Data: map[string]any{
					"name":    name,
					"sources": currentSources,
				},
			})
		}

		knownStreams[name] = currentSources
	}

	// Check for removed streams
	for name := range knownStreams {
		found := false
		for _, currentName := range currentNames {
			if currentName == name {
				found = true
				break
			}
		}

		if !found {
			eventBus.Publish(Event{
				Type:      "stream_removed",
				Timestamp: time.Now(),
				Data: map[string]any{
					"name": name,
				},
			})
			delete(knownStreams, name)
		}
	}
}

func sourcesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]struct{})
	for _, s := range a {
		aMap[s] = struct{}{}
	}

	for _, s := range b {
		if _, exists := aMap[s]; !exists {
			return false
		}
	}

	return true
}

// GetRecentEvents returns a list of recent events for MCP tools
func GetRecentEvents(count int) []Event {
	// This is a simplified version - in production, you'd want to store
	// events in a circular buffer or similar structure
	ch := eventBus.Subscribe()
	defer eventBus.Unsubscribe(ch)

	events := make([]Event, 0, count)
	deadline := time.After(time.Second)

	for i := 0; i < count; i++ {
		select {
		case event := <-ch:
			events = append(events, event)
		case <-deadline:
			break
		}
	}

	return events
}

// Event logging for resources
type EventLog struct {
	events []Event
	mu     sync.RWMutex
	limit  int
}

var eventLog = &EventLog{
	events: make([]Event, 0, 100),
	limit:  100,
}

func (el *EventLog) Add(event Event) {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.events = append(el.events, event)
	if len(el.events) > el.limit {
		// Remove oldest events
		el.events = el.events[len(el.events)-el.limit:]
	}
}

func (el *EventLog) Get(count int) []Event {
	el.mu.RLock()
	defer el.mu.RUnlock()

	if count > len(el.events) {
		count = len(el.events)
	}

	start := len(el.events) - count
	if start < 0 {
		start = 0
	}

	result := make([]Event, count)
	copy(result, el.events[start:])
	return result
}

// LogEvent stores an event in the event log
func LogEvent(eventType string, data any) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
	eventLog.Add(event)
	eventBus.Publish(event)
}
