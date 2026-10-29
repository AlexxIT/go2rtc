package streams

import (
	"fmt"
	"maps"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/probe"
)

type Preload struct {
	stream *Stream      // Don't include the stream in JSON to avoid leaking secrets.
	Cons   *probe.Probe `json:"consumer"`
	Query  string       `json:"query"`
}

var preloads = map[string]*Preload{}
var preloadsMu sync.Mutex

func AddPreload(name, rawQuery string) error {
	if rawQuery == "" {
		rawQuery = "video&audio"
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return err
	}

	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	if p := preloads[name]; p != nil {
		p.stream.RemoveConsumer(p.Cons)
	}

	stream := Get(name)
	if stream == nil {
		return fmt.Errorf("streams: stream not found: %s", name)
	}
	cons := probe.Create("preload", query)

	if err = stream.AddConsumer(cons); err != nil {
		return err
	}

	preloads[name] = &Preload{stream: stream, Cons: cons, Query: rawQuery}
	return nil
}

func DelPreload(name string) error {
	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	if p := preloads[name]; p != nil {
		p.stream.RemoveConsumer(p.Cons)
		delete(preloads, name)
		return nil
	}

	return fmt.Errorf("streams: preload not found: %s", name)
}

func GetPreloads() map[string]*Preload {
	preloadsMu.Lock()
	defer preloadsMu.Unlock()
	return maps.Clone(preloads)
}

// preloadRetryDelays is the back-off between attempts to bring a preload up
// when its source is not ready: throttled by a rate limiter, a camera that is
// switched off, a server that is down. The last delay repeats forever.
var preloadRetryDelays = []time.Duration{
	15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute,
}

func preloadRetryDelay(attempt int) time.Duration {
	if attempt >= len(preloadRetryDelays) {
		attempt = len(preloadRetryDelays) - 1
	}
	return preloadRetryDelays[attempt]
}

// PreloadWithRetry adds a preload and, if the source cannot be dialed yet,
// keeps trying on a back-off until it can. Before this a preload that failed
// at startup - a dozen cameras dialed at once against a per-minute quota is
// enough - stayed cold until some other consumer happened to dial the stream.
func PreloadWithRetry(name, rawQuery string) {
	preloadWithRetry(name, rawQuery, 0)
}

func preloadWithRetry(name, rawQuery string, attempt int) {
	err := AddPreload(name, rawQuery)
	if err == nil {
		if attempt > 0 {
			log.Info().Msgf("[streams] preload %s is up after %d retries", name, attempt)
		}
		return
	}
	delay := preloadRetryDelay(attempt)
	log.Warn().Err(err).Msgf("[streams] preload %s failed, retrying in %s", name, delay)
	time.AfterFunc(delay, func() { preloadWithRetry(name, rawQuery, attempt+1) })
}
