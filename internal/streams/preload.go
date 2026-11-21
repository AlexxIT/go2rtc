package streams

import (
	"fmt"
	"maps"
	"net/url"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/probe"
)

type Preload struct {
	stream *Stream      // Don't include the stream in JSON to avoid leaking secrets.
	Cons   *probe.Probe `json:"consumer"`
	Query  string       `json:"query"`
}

var preloads = map[string]*Preload{}
var preloadsMu sync.Mutex

type preloadLock struct {
	mu   sync.Mutex
	refs int
}

var preloadLocks = map[string]*preloadLock{}

// Serialize changes to one preload without holding the registry lock during
// producer dialing or shutdown. Count waiting callers so they share the lock.
func lockPreload(name string) func() {
	preloadsMu.Lock()
	lock := preloadLocks[name]
	if lock == nil {
		lock = &preloadLock{}
		preloadLocks[name] = lock
	}
	lock.refs++
	preloadsMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		preloadsMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(preloadLocks, name)
		}
		preloadsMu.Unlock()
	}
}

func AddPreload(name, rawQuery string) error {
	if rawQuery == "" {
		rawQuery = "video&audio"
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return err
	}

	unlock := lockPreload(name)
	defer unlock()

	preloadsMu.Lock()
	p := preloads[name]
	preloadsMu.Unlock()
	if p != nil {
		p.stream.preloadMu.Lock()
		p.stream.RemoveConsumer(p.Cons)
		p.stream.preloadMu.Unlock()
	}

	stream := Get(name)
	if stream == nil {
		return fmt.Errorf("streams: stream not found: %s", name)
	}
	cons := probe.Create("preload", query)

	stream.preloadMu.Lock()
	err = stream.AddConsumer(cons)
	stream.preloadMu.Unlock()
	if err != nil {
		return err
	}

	preloadsMu.Lock()
	preloads[name] = &Preload{stream: stream, Cons: cons, Query: rawQuery}
	preloadsMu.Unlock()
	return nil
}

func DelPreload(name string) error {
	unlock := lockPreload(name)
	defer unlock()

	preloadsMu.Lock()
	p := preloads[name]
	preloadsMu.Unlock()

	if p != nil {
		p.stream.preloadMu.Lock()
		p.stream.RemoveConsumer(p.Cons)
		p.stream.preloadMu.Unlock()
		preloadsMu.Lock()
		delete(preloads, name)
		preloadsMu.Unlock()
		return nil
	}

	return fmt.Errorf("streams: preload not found: %s", name)
}

func GetPreloads() map[string]*Preload {
	preloadsMu.Lock()
	defer preloadsMu.Unlock()
	return maps.Clone(preloads)
}
