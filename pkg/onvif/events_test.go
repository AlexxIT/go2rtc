package onvif

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func wellFormed(t *testing.T, b []byte) {
	d := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		if _, err := d.Token(); err != nil {
			require.Equal(t, "EOF", err.Error(), string(b))
			return
		}
	}
}

func pull(t *testing.T, e *Events, id string, timeout time.Duration) []byte {
	b, err := e.Pull(context.Background(), id, timeout, 10)
	require.NoError(t, err)
	return b
}

func TestParseDuration(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.Equal(t, 2*time.Second, ParseDuration("PT2S", now))
	require.Equal(t, time.Minute, ParseDuration("PT1M", now))
	require.Equal(t, 90*time.Minute, ParseDuration("PT1H30M", now))
	require.Equal(t, 24*time.Hour, ParseDuration("P1D", now))
	require.Equal(t, 1500*time.Millisecond, ParseDuration("PT1.5S", now))
	require.Equal(t, time.Minute, ParseDuration("2026-01-02T03:05:05Z", now))
	require.Equal(t, time.Duration(0), ParseDuration("", now))
	require.Equal(t, time.Duration(0), ParseDuration("garbage", now))
}

func TestSubscriptionID(t *testing.T) {
	require.Equal(t, "abc", SubscriptionID("/onvif/subscription/abc"))
	require.Equal(t, "", SubscriptionID("/onvif/events_service"))
}

func TestEventsResponses(t *testing.T) {
	wellFormed(t, GetEventPropertiesResponse())
	wellFormed(t, EventsServiceCapabilitiesResponse())
	wellFormed(t, CreatePullPointSubscriptionResponse("localhost:1984", "abc", time.Now()))
	wellFormed(t, RenewResponse(time.Now()))
	wellFormed(t, UnsubscribeResponse())
	wellFormed(t, SetSynchronizationPointResponse())
	wellFormed(t, ResourceUnknownResponse())
	wellFormed(t, CreatePullPointSubscriptionResponse(`a"b<c>`, "abc", time.Now()))

	b := GetEventPropertiesResponse()
	require.Contains(t, string(b), "CellMotionDetector")
	require.Contains(t, string(b), "MotionAlarm")
}

func TestEventsPull(t *testing.T) {
	e := NewEvents()
	require.False(t, e.HasSources())
	e.AddSource("cam")
	require.True(t, e.HasSources())

	id, expires, ok := e.Subscribe(time.Minute)
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Minute), expires, time.Second)

	// 1. initial state
	b := pull(t, e, id, time.Second)
	wellFormed(t, b)
	s := string(b)
	require.Contains(t, s, `PropertyOperation="Initialized"`)
	require.Contains(t, s, `Name="IsMotion" Value="false"`)
	require.Contains(t, s, `Name="State" Value="false"`)
	require.Contains(t, s, `Name="VideoSourceConfigurationToken" Value="cam"`)

	// 2. timeout without messages
	start := time.Now()
	b = pull(t, e, id, 100*time.Millisecond)
	wellFormed(t, b)
	require.NotContains(t, string(b), "NotificationMessage")
	require.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)

	// 3. long poll wakes up on motion
	go func() {
		time.Sleep(50 * time.Millisecond)
		e.SetMotion("cam", true)
	}()
	start = time.Now()
	b = pull(t, e, id, 5*time.Second)
	require.Less(t, time.Since(start), time.Second)
	require.Contains(t, string(b), `PropertyOperation="Changed"`)
	require.Contains(t, string(b), `Name="IsMotion" Value="true"`)

	// 4. same state - no message, new state - message
	e.SetMotion("cam", true)
	e.SetMotion("cam", false)
	b = pull(t, e, id, time.Second)
	require.Equal(t, 2, strings.Count(string(b), "<wsnt:NotificationMessage>"))
	require.Contains(t, string(b), `Name="IsMotion" Value="false"`)

	// 5. sync point
	require.True(t, e.Sync(id))
	b = pull(t, e, id, time.Second)
	require.Contains(t, string(b), `PropertyOperation="Initialized"`)

	// 6. renew and unsubscribe
	_, ok = e.Renew(id, time.Minute)
	require.True(t, ok)
	require.True(t, e.Unsubscribe(id))
	require.False(t, e.Unsubscribe(id))
	_, err := e.Pull(context.Background(), id, time.Second, 10)
	require.ErrorIs(t, err, ErrUnknownSubscription)
	_, ok = e.Renew(id, time.Minute)
	require.False(t, ok)
}

func TestEventsExpire(t *testing.T) {
	e := NewEvents()
	e.AddSource("cam")

	id, _, ok := e.Subscribe(time.Minute)
	require.True(t, ok)

	e.mu.Lock()
	e.subs[id].expires = time.Now().Add(-time.Second)
	e.mu.Unlock()

	_, err := e.Pull(context.Background(), id, time.Second, 10)
	require.ErrorIs(t, err, ErrUnknownSubscription)
}

func TestEventsLimits(t *testing.T) {
	e := NewEvents()
	for i := 0; i < subscriptionsMax; i++ {
		_, _, ok := e.Subscribe(time.Minute)
		require.True(t, ok)
	}
	_, _, ok := e.Subscribe(time.Minute)
	require.False(t, ok)
}

func TestEventsEscape(t *testing.T) {
	e := NewEvents()
	e.AddSource(`cam"<&>`)

	id, _, ok := e.Subscribe(time.Minute)
	require.True(t, ok)

	b := pull(t, e, id, time.Second)
	wellFormed(t, b)
	require.Contains(t, string(b), `Value="cam&#34;&lt;&amp;&gt;"`)
}

func TestEventsPullCancel(t *testing.T) {
	e := NewEvents()

	// unsubscribe wakes up the pending pull
	id, _, _ := e.Subscribe(time.Minute)
	go func() {
		time.Sleep(50 * time.Millisecond)
		e.Unsubscribe(id)
	}()
	start := time.Now()
	_, err := e.Pull(context.Background(), id, 5*time.Second, 10)
	require.ErrorIs(t, err, ErrUnknownSubscription)
	require.Less(t, time.Since(start), time.Second)

	// client disconnect ends the pending pull
	id, _, _ = e.Subscribe(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start = time.Now()
	_, err = e.Pull(ctx, id, 5*time.Second, 10)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), time.Second)
}

func TestEventsSetMotionAddsSource(t *testing.T) {
	e := NewEvents()
	e.SetMotion("cam", false)
	require.True(t, e.HasSources())

	id, _, _ := e.Subscribe(time.Minute)
	b := pull(t, e, id, time.Second)
	require.Contains(t, string(b), `Name="Source" Value="cam"`)
}
