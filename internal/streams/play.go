package streams

import (
	"errors"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// playMixer is implemented by consumers that can play several sources at
// the same time. Other consumers get only one, a new playback replaces the
// running one.
type playMixer interface {
	CanMix() bool
}

// playClearer is implemented by consumers that queue the audio of a
// playback. ClearPlayback drops the queue when the playback is stopped.
type playClearer interface {
	ClearPlayback()
}

func canMix(cons core.Consumer) bool {
	mixer, ok := cons.(playMixer)
	return ok && mixer.CanMix()
}

// playSession is a connection to the camera that was opened only for
// playback. All playbacks that run at the same time share it, because most
// cameras support only one talk session.
type playSession struct {
	dst     core.Producer // the same connection as cons
	cons    core.Consumer
	srcs    []core.Producer
	closing bool          // no more sources can be added
	done    chan struct{} // closed when the connection to the camera is closed
	once    sync.Once
}

// closePlaySession closes the connection to the camera and lets Play continue
func (s *Stream) closePlaySession(ps *playSession) {
	ps.once.Do(func() {
		_ = ps.dst.Stop()

		s.mu.Lock()
		if s.play == ps {
			s.play = nil
		}
		s.mu.Unlock()

		close(ps.done)
	})
}

// stopPlayback stops all running playbacks and drops the audio that is
// already queued for the camera.
func (s *Stream) stopPlayback() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, producer := range s.producers {
		if producer.conn == nil {
			continue
		}
		if producer.state == stateInternal {
			_ = producer.conn.Stop()
		} else if clearer, ok := producer.conn.(playClearer); ok {
			clearer.ClearPlayback()
		}
	}

	if s.play == nil {
		return
	}
	if clearer, ok := s.play.cons.(playClearer); ok {
		clearer.ClearPlayback()
	}
}

// Play starts a playback to the stream. An empty source stops all playbacks.
//
// If the camera connection can mix (see playMixer), a new playback is added
// to the running ones. Otherwise it replaces them.
func (s *Stream) Play(urlOrProd any) error {
	s.playMu.Lock()
	defer s.playMu.Unlock()

	var source string
	var src core.Producer

	switch urlOrProd.(type) {
	case string:
		if source = urlOrProd.(string); source == "" {
			s.stopPlayback()
			return nil
		}
	case core.Producer:
		src = urlOrProd.(core.Producer)
	}

	s.mu.Lock()
	ps := s.play
	closing := ps != nil && ps.closing
	s.mu.Unlock()

	if ps != nil && !closing && !canMix(ps.cons) {
		// only one source at a time, so replace the running one
		s.stopPlayback()

		s.mu.Lock()
		ps.closing = true
		s.mu.Unlock()

		go s.closePlaySession(ps)
		closing = true
	}

	if closing {
		// Wait until the connection to the camera is closed. A talk session
		// that is started too early is rejected by the camera.
		select {
		case <-ps.done:
		case <-time.After(15 * time.Second):
		}
		ps = nil
	}

	// 1. Running playback session
	if ps != nil {
		if src == nil {
			var err error
			if src, err = GetProducer(source); err != nil {
				return err
			}
		}

		if !matchMedia(src, ps.cons) {
			// a second connection would get no talk session and the running
			// session would be lost for stopPlayback
			return errors.New("streams: playback doesn't fit the running one")
		}

		s.addPlaySource(ps, src)
		return nil
	}

	// 2. Active producer of the stream with backchannel
	for _, producer := range s.producers {
		if producer.conn == nil || producer.state == stateInternal {
			continue
		}

		cons, ok := producer.conn.(core.Consumer)
		if !ok {
			continue
		}

		if !canMix(cons) {
			s.stopPlayback()
		}

		if src == nil {
			var err error
			if src, err = GetProducer(source); err != nil {
				return err
			}
		}

		if !matchMedia(src, cons) {
			continue
		}

		// stopProducers has to keep this connection while the playback runs
		conn := producer.conn

		s.mu.Lock()
		if s.playConns == nil {
			s.playConns = map[core.Producer]int{}
		}
		s.playConns[conn]++
		s.mu.Unlock()

		s.AddInternalProducer(src)

		go func() {
			_ = src.Start()

			s.mu.Lock()
			if s.playConns[conn]--; s.playConns[conn] <= 0 {
				delete(s.playConns, conn)
			}
			s.mu.Unlock()

			s.RemoveProducer(src)
			s.stopProducers()
		}()

		return nil
	}

	// 3. New connection to the camera only for playback
	for _, producer := range s.producers {
		if producer.state == stateInternal {
			continue
		}

		// start new client
		dst, err := GetProducer(producer.url)
		if err != nil {
			continue
		}

		// check if client support consumer interface
		cons, ok := dst.(core.Consumer)
		if !ok {
			_ = dst.Stop()
			continue
		}

		// start new producer
		if src == nil {
			if src, err = GetProducer(source); err != nil {
				_ = dst.Stop()
				return err
			}
		}

		if !matchMedia(src, cons) {
			_ = dst.Stop()
			continue
		}

		ps = &playSession{dst: dst, cons: cons, done: make(chan struct{})}

		s.mu.Lock()
		s.play = ps
		s.mu.Unlock()

		s.AddInternalConsumer(cons)
		s.addPlaySource(ps, src)

		go func() {
			// returns when the connection to the camera is closed
			_ = dst.Start()

			s.mu.Lock()
			ps.closing = true
			srcs := append([]core.Producer(nil), ps.srcs...)
			s.mu.Unlock()

			for _, src := range srcs {
				_ = src.Stop()
			}
			s.RemoveInternalConsumer(cons)
			s.closePlaySession(ps)
		}()

		return nil
	}

	return errors.New("can't find consumer")
}

// addPlaySource starts the source and closes the session after the last one
func (s *Stream) addPlaySource(ps *playSession, src core.Producer) {
	s.mu.Lock()
	ps.srcs = append(ps.srcs, src)
	s.mu.Unlock()

	s.AddInternalProducer(src)

	go func() {
		_ = src.Start()

		// Play adds a source only to a session that is not closing, so it
		// must not run while we check for the last source
		s.playMu.Lock()
		s.mu.Lock()
		for i, v := range ps.srcs {
			if v == src {
				ps.srcs = append(ps.srcs[:i], ps.srcs[i+1:]...)
				break
			}
		}
		last := len(ps.srcs) == 0 && !ps.closing
		if last {
			ps.closing = true
		}
		s.mu.Unlock()
		s.playMu.Unlock()

		if last {
			// little timeout before stop dst, so the buffer can be transferred
			time.Sleep(time.Second)
			s.closePlaySession(ps)
		}

		s.RemoveProducer(src)
		s.stopProducers()
	}()
}

func (s *Stream) AddInternalProducer(conn core.Producer) {
	producer := &Producer{conn: conn, state: stateInternal, url: "internal"}
	s.mu.Lock()
	s.producers = append(s.producers, producer)
	s.mu.Unlock()
}

func (s *Stream) AddInternalConsumer(conn core.Consumer) {
	s.mu.Lock()
	s.consumers = append(s.consumers, conn)
	s.mu.Unlock()
}

func (s *Stream) RemoveInternalConsumer(conn core.Consumer) {
	s.mu.Lock()
	for i, consumer := range s.consumers {
		if consumer == conn {
			s.consumers = append(s.consumers[:i], s.consumers[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

func matchMedia(prod core.Producer, cons core.Consumer) bool {
	for _, consMedia := range cons.GetMedias() {
		for _, prodMedia := range prod.GetMedias() {
			if prodMedia.Direction != core.DirectionRecvonly {
				continue
			}

			prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
			if prodCodec == nil {
				continue
			}

			track, err := prod.GetTrack(prodMedia, prodCodec)
			if err != nil {
				log.Warn().Err(err).Msg("[streams] can't get track")
				continue
			}

			if err = cons.AddTrack(consMedia, consCodec, track); err != nil {
				log.Warn().Err(err).Msg("[streams] can't add track")
				continue
			}

			return true
		}
	}

	return false
}
