package streams

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/magic"
)

type SnapshotCacher struct {
	consumer      *magic.Keyframe
	stream        *Stream
	idleTimer     *time.Timer
	idleTimeout   time.Duration
	stopped       atomic.Bool
	transcodeFunc func([]byte, string) ([]byte, error) // injected from mjpeg module
}

// GetCachedSnapshot returns cached JPEG and timestamp (if available)
func (s *Stream) GetCachedSnapshot() (data []byte, timestamp time.Time, exists bool) {
	s.cachedJPEGMu.RLock()
	defer s.cachedJPEGMu.RUnlock()

	if s.cachedJPEG == nil {
		return nil, time.Time{}, false
	}

	return s.cachedJPEG, s.cachedJPEGTime, true
}

// TouchSnapshotCache starts cacher if needed, resets idle timer
func (s *Stream) TouchSnapshotCache(timeout time.Duration, transcodeFunc func([]byte, string) ([]byte, error)) {
	if timeout < 0 {
		return // caching disabled
	}

	s.snapshotCacherMu.Lock()
	defer s.snapshotCacherMu.Unlock()

	if s.snapshotCacher != nil {
		// Already running, just reset idle timer
		s.snapshotCacher.resetIdleTimer()
		return
	}

	// Start new background cacher
	s.snapshotCacher = s.startSnapshotCacher(timeout, transcodeFunc)
}

func (s *Stream) startSnapshotCacher(timeout time.Duration, transcodeFunc func([]byte, string) ([]byte, error)) *SnapshotCacher {
	cons := magic.NewKeyframe()

	cacher := &SnapshotCacher{
		consumer:      cons,
		stream:        s,
		idleTimeout:   timeout,
		transcodeFunc: transcodeFunc,
	}

	// Set up idle timer (0 = never timeout)
	if timeout > 0 {
		cacher.idleTimer = time.AfterFunc(timeout, func() {
			cacher.stop()
		})
	}

	// Add as persistent consumer
	// This may trigger producer start, or piggyback on existing connection
	if err := s.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msg("[snapshot-cache] failed to add consumer")
		return nil
	}

	// Start background processing loop
	go cacher.run()

	log.Debug().Msg("[snapshot-cache] started")

	return cacher
}

func (c *SnapshotCacher) run() {
	defer log.Debug().Msg("[snapshot-cache] stopped")

	// Use custom writer that updates cache on each keyframe write
	cacheWriter := &snapshotCacheWriter{cacher: c}

	// WriteTo will block here and stream keyframes continuously
	// until the connection closes or an error occurs
	_, err := c.consumer.WriteTo(cacheWriter)

	if err != nil && !c.stopped.Load() {
		log.Warn().Err(err).Msg("[snapshot-cache] consumer error")
	}

	// Clean up on exit
	c.stop()
}

// snapshotCacheWriter implements io.Writer and updates cache on each Write
type snapshotCacheWriter struct {
	cacher *SnapshotCacher
}

func (w *snapshotCacheWriter) Write(b []byte) (n int, err error) {
	if w.cacher.stopped.Load() {
		return 0, io.ErrClosedPipe
	}

	// Make a copy since b may be reused by the producer
	frame := make([]byte, len(b))
	copy(frame, b)

	// Transcode to JPEG if needed using injected function
	codecName := w.cacher.consumer.CodecName()
	if w.cacher.transcodeFunc != nil {
		frame, err = w.cacher.transcodeFunc(frame, codecName)
		if err != nil {
			log.Warn().Err(err).Str("codec", codecName).Msg("[snapshot-cache] transcode failed")
			return len(b), nil // Return success to continue receiving frames
		}
	} else {
		log.Warn().Str("codec", codecName).Msg("[snapshot-cache] no transcode function")
		return len(b), nil
	}

	// Update cache atomically
	w.cacher.stream.cachedJPEGMu.Lock()
	w.cacher.stream.cachedJPEG = frame
	w.cacher.stream.cachedJPEGTime = time.Now()
	w.cacher.stream.cachedJPEGMu.Unlock()

	log.Debug().
		Str("codec", codecName).
		Int("size", len(frame)).
		Msg("[snapshot-cache] updated")

	return len(b), nil
}

func (c *SnapshotCacher) resetIdleTimer() {
	if c.idleTimer != nil && !c.stopped.Load() {
		c.idleTimer.Reset(c.idleTimeout)
		log.Trace().Msg("[snapshot-cache] idle timer reset")
	}
}

func (c *SnapshotCacher) stop() {
	if !c.stopped.CompareAndSwap(false, true) {
		return
	}

	log.Debug().Msg("[snapshot-cache] stopping")

	// Stop the timer
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}

	// Close the consumer transport to unblock WriteTo
	if err := c.consumer.Stop(); err != nil {
		log.Trace().Err(err).Msg("[snapshot-cache] consumer close error")
	}

	// Remove consumer (may stop producer if no other consumers)
	c.stream.RemoveConsumer(c.consumer)

	// Clear the cacher reference (but keep cached JPEG in memory!)
	c.stream.snapshotCacherMu.Lock()
	c.stream.snapshotCacher = nil
	c.stream.snapshotCacherMu.Unlock()
}