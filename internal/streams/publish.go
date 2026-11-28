package streams

import (
	"context"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// publishHandle tracks one active publish. Closing cons unblocks an in-flight
// run() so a stop takes effect at once; cancel and cons are guarded by Stream.mu.
type publishHandle struct {
	cancel context.CancelFunc
	cons   core.Consumer
}

func (s *Stream) Publish(url string) error {
	s.StopPublish(url) // replace any existing publish for this url

	ctx, cancel := context.WithCancel(context.Background())
	h := &publishHandle{cancel: cancel}

	s.mu.Lock()
	if s.publishes == nil {
		s.publishes = make(map[string]*publishHandle)
	}
	s.publishes[url] = h
	s.mu.Unlock()

	cons, run, err := GetConsumer(url)
	if err != nil {
		cancel()
		s.removeHandle(url, h)
		return err
	}

	if err = s.AddConsumer(cons); err != nil {
		cancel()
		s.removeHandle(url, h)
		return err
	}

	go func() {
		defer s.removeHandle(url, h)

		retry := 0
		for {
			// register cons under mu, or bail if already cancelled, so a stop can't miss it
			s.mu.Lock()
			if ctx.Err() != nil {
				s.mu.Unlock()
				s.RemoveConsumer(cons)
				return
			}
			h.cons = cons
			s.mu.Unlock()

			run()
			s.RemoveConsumer(cons)

			// exponential backoff, same schedule as producer.go reconnect
			timeout := time.Minute
			if retry < 5 {
				timeout = time.Second
			} else if retry < 10 {
				timeout = time.Second * 5
			} else if retry < 20 {
				timeout = time.Second * 10
			}
			retry++

			log.Debug().Msgf("[streams] publish retry=%d timeout=%s url=%s", retry, timeout, url)

			select {
			case <-ctx.Done():
				log.Debug().Msgf("[streams] publish cancelled url=%s", url)
				return
			case <-time.After(timeout):
			}

			cons, run, err = GetConsumer(url)
			if err != nil {
				log.Warn().Err(err).Msgf("[streams] publish permanent error, stopping url=%s", url)
				return
			}
			if err = s.AddConsumer(cons); err != nil {
				log.Debug().Err(err).Msgf("[streams] publish add consumer failed url=%s", url)
				continue
			}
		}
	}()

	return nil
}

// removeHandle drops h only if it's still the current handle, so a fast republish
// that already installed a new handle isn't clobbered.
func (s *Stream) removeHandle(url string, h *publishHandle) {
	s.mu.Lock()
	if s.publishes[url] == h {
		delete(s.publishes, url)
	}
	s.mu.Unlock()
}

// StopPublish cancels the publish goroutine for url and closes its live consumer,
// so an in-flight run() unblocks at once instead of lingering until the remote drops.
func (s *Stream) StopPublish(url string) {
	s.mu.Lock()
	h, ok := s.publishes[url]
	var cons core.Consumer
	if ok {
		h.cancel() // cancel under mu, serialized with the goroutine's ctx check
		cons = h.cons
		delete(s.publishes, url)
	}
	s.mu.Unlock()

	if !ok {
		return
	}
	if cons != nil {
		s.RemoveConsumer(cons) // RemoveConsumer takes mu, so call it unlocked
	}
	log.Info().Msgf("[streams] stopped publish url=%s", url)
}

// StopAllPublish stops every active publish on this stream.
func (s *Stream) StopAllPublish() {
	s.mu.Lock()
	conss := make([]core.Consumer, 0, len(s.publishes))
	for _, h := range s.publishes {
		h.cancel()
		if h.cons != nil {
			conss = append(conss, h.cons)
		}
	}
	s.publishes = make(map[string]*publishHandle)
	s.mu.Unlock()

	for _, cons := range conss {
		s.RemoveConsumer(cons)
	}
}

func Publish(stream *Stream, destination any) {
	switch v := destination.(type) {
	case string:
		if err := stream.Publish(v); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	case []any:
		for _, v := range v {
			Publish(stream, v)
		}
	}
}
