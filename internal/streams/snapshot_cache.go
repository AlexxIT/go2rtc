package streams

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/magic"
)

type SnapshotCacher struct {
	name          string // stream name for logging
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
func (s *Stream) TouchSnapshotCache(name string, timeout time.Duration, transcodeFunc func([]byte, string) ([]byte, error)) {
	if timeout < 0 {
		return // caching disabled
	}

	// Check cache age BEFORE acquiring snapshotCacherMu to avoid deadlock
	s.cachedJPEGMu.RLock()
	cacheAge := time.Since(s.cachedJPEGTime)
	s.cachedJPEGMu.RUnlock()

	s.snapshotCacherMu.Lock()
	defer s.snapshotCacherMu.Unlock()

	if s.snapshotCacher != nil {
		maxStaleTime := timeout * 2
		if maxStaleTime > 0 && cacheAge > maxStaleTime {
			log.Warn().Msgf("[snapshot-cache] touch: stream=%s cache is stale (age=%v > max=%v), forcing restart",
				name, cacheAge, maxStaleTime)
			s.snapshotCacher.stop()
			s.snapshotCacher = nil
			// Fall through to start new cacher
		} else {
			// Already running and healthy, just reset idle timer
			log.Trace().Msgf("[snapshot-cache] touch: stream=%s already running (cache age=%v), resetting timer", name, cacheAge)
			s.snapshotCacher.resetIdleTimer()
			return
		}
	}

	// Start new background cacher (will retry on every request if nil)
	log.Debug().Msgf("[snapshot-cache] touch: stream=%s cacher is nil, attempting to start", name)
	s.snapshotCacher = s.startSnapshotCacher(name, timeout, transcodeFunc)
	if s.snapshotCacher == nil {
		log.Warn().Msgf("[snapshot-cache] touch: stream=%s failed to start cacher, will retry on next request", name)
		// Leave s.snapshotCacher as nil so next request will retry
		// Old cached snapshot remains available
	} else {
		log.Info().Msgf("[snapshot-cache] touch: stream=%s successfully started cacher", name)
	}
}

func (s *Stream) startSnapshotCacher(name string, timeout time.Duration, transcodeFunc func([]byte, string) ([]byte, error)) *SnapshotCacher {
	cons := magic.NewKeyframe()

	cacher := &SnapshotCacher{
		name:          name,
		consumer:      cons,
		stream:        s,
		idleTimeout:   timeout,
		transcodeFunc: transcodeFunc,
	}

	log.Debug().Msgf("[snapshot-cache] stream=%s creating cacher with timeout=%v", name, timeout)

	// Set up idle timer (0 = never timeout)
	if timeout > 0 {
		cacher.idleTimer = time.AfterFunc(timeout, func() {
			log.Debug().Msgf("[snapshot-cache] stream=%s idle timeout reached, stopping", name)
			cacher.stop()
		})
	}

	// Add as persistent consumer
	// This may trigger producer start, or piggyback on existing connection
	log.Debug().Msgf("[snapshot-cache] stream=%s attempting to add consumer", name)
	if err := s.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msgf("[snapshot-cache] stream=%s failed to add consumer", name)
		return nil
	}

	// Start background processing loop
	go cacher.run()

	log.Debug().Msgf("[snapshot-cache] stream=%s started successfully", name)

	return cacher
}

func (c *SnapshotCacher) run() {
	log.Debug().Msgf("[snapshot-cache] stream=%s run loop starting", c.name)
	defer log.Debug().Msgf("[snapshot-cache] stream=%s run loop exiting", c.name)

	// Use custom writer that updates cache on each keyframe write
	cacheWriter := &snapshotCacheWriter{cacher: c}

	// WriteTo will block here and stream keyframes continuously
	// until the connection closes or an error occurs
	log.Debug().Msgf("[snapshot-cache] stream=%s calling consumer.WriteTo", c.name)
	bytesWritten, err := c.consumer.WriteTo(cacheWriter)

	if err != nil && !c.stopped.Load() {
		log.Warn().Err(err).Msgf("[snapshot-cache] stream=%s consumer error after %d bytes", c.name, bytesWritten)
	} else {
		log.Debug().Msgf("[snapshot-cache] stream=%s consumer.WriteTo completed normally, bytes=%d", c.name, bytesWritten)
	}

	// Clean up on exit
	c.stop()

	// Clear the cacher reference so it can be restarted
	c.stream.snapshotCacherMu.Lock()
	c.stream.snapshotCacher = nil
	c.stream.snapshotCacherMu.Unlock()
}

// snapshotCacheWriter implements io.Writer and updates cache on each Write
type snapshotCacheWriter struct {
	cacher *SnapshotCacher
}

func (w *snapshotCacheWriter) Write(b []byte) (n int, err error) {
	if w.cacher.stopped.Load() {
		log.Trace().Msgf("[snapshot-cache] stream=%s write called but cacher stopped", w.cacher.name)
		return 0, io.ErrClosedPipe
	}

	log.Trace().Msgf("[snapshot-cache] stream=%s received frame size=%d", w.cacher.name, len(b))

	// Make a copy since b may be reused by the producer
	frame := make([]byte, len(b))
	copy(frame, b)

	// Transcode to JPEG if needed using injected function
	codecName := w.cacher.consumer.CodecName()
	log.Trace().Msgf("[snapshot-cache] stream=%s codec=%s, transcoding", w.cacher.name, codecName)

	if w.cacher.transcodeFunc != nil {
		frame, err = w.cacher.transcodeFunc(frame, codecName)
		if err != nil {
			log.Warn().Err(err).Msgf("[snapshot-cache] stream=%s codec=%s transcode failed", w.cacher.name, codecName)
			return len(b), nil // Return success to continue receiving frames
		}
		log.Trace().Msgf("[snapshot-cache] stream=%s transcode successful, jpeg size=%d", w.cacher.name, len(frame))
	} else {
		log.Warn().Msgf("[snapshot-cache] stream=%s codec=%s no transcode function", w.cacher.name, codecName)
		return len(b), nil
	}

	// Update cache atomically
	timestamp := time.Now()
	w.cacher.stream.cachedJPEGMu.Lock()
	w.cacher.stream.cachedJPEG = frame
	w.cacher.stream.cachedJPEGTime = timestamp
	w.cacher.stream.cachedJPEGMu.Unlock()

	log.Debug().Msgf("[snapshot-cache] stream=%s codec=%s updated cache, size=%d, timestamp=%s",
		w.cacher.name, codecName, len(frame), timestamp.Format(time.RFC3339Nano))

	return len(b), nil
}

func (c *SnapshotCacher) resetIdleTimer() {
	if c.idleTimer != nil && !c.stopped.Load() {
		c.idleTimer.Reset(c.idleTimeout)
		log.Trace().Msgf("[snapshot-cache] stream=%s idle timer reset to %v", c.name, c.idleTimeout)
	}
}

func (c *SnapshotCacher) stop() {
	if !c.stopped.CompareAndSwap(false, true) {
		log.Trace().Msgf("[snapshot-cache] stream=%s stop called but already stopped", c.name)
		return
	}

	log.Debug().Msgf("[snapshot-cache] stream=%s stopping", c.name)

	// Stop the timer
	if c.idleTimer != nil {
		log.Debug().Msgf("[snapshot-cache] stream=%s stopping idle timer", c.name)
		c.idleTimer.Stop()
	}

	// Close the consumer transport to unblock WriteTo
	log.Debug().Msgf("[snapshot-cache] stream=%s stopping consumer", c.name)
	if err := c.consumer.Stop(); err != nil {
		log.Trace().Err(err).Msgf("[snapshot-cache] stream=%s consumer close error", c.name)
	}

	// Remove consumer (may stop producer if no other consumers)
	log.Debug().Msgf("[snapshot-cache] stream=%s removing consumer from stream", c.name)
	c.stream.RemoveConsumer(c.consumer)

	log.Debug().Msgf("[snapshot-cache] stream=%s stop complete, cached snapshot retained in memory", c.name)
}

// stopAndClear stops the cacher and clears the reference - MUST be called with snapshotCacherMu held
func (c *SnapshotCacher) stopAndClear() {
	c.stop()
	// Caller must hold snapshotCacherMu and clear c.stream.snapshotCacher = nil
}