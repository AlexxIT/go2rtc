package homekit

import (
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hksv"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/rs/zerolog"
)

const (
	onvifSubscriptionTimeout = 60 * time.Second
	onvifPullTimeout         = 30 * time.Second
	onvifMessageLimit        = 10
	onvifRenewMargin         = 10 * time.Second
	onvifMinReconnectDelay   = 5 * time.Second
	onvifMaxReconnectDelay   = 60 * time.Second
)

type onvifPullPoint interface {
	PullMessages(timeout time.Duration, limit int) ([]byte, error)
	Renew(timeout time.Duration) error
	Unsubscribe() error
}

type onvifPullPointFactory func(rawURL string, timeout time.Duration) (onvifPullPoint, error)

// onvifMotionWatcher subscribes to ONVIF PullPoint events
// and forwards motion state to an hksv.Server.
type onvifMotionWatcher struct {
	srv      *hksv.Server
	onvifURL string
	holdTime time.Duration
	log      zerolog.Logger

	now                 func() time.Time
	newPullPoint        onvifPullPointFactory
	subscriptionTimeout time.Duration
	pullTimeout         time.Duration
	renewMargin         time.Duration
	messageLimit        int

	done chan struct{}
	once sync.Once
}

func newOnvifMotionWatcher(srv *hksv.Server, onvifURL string, holdTime time.Duration, log zerolog.Logger) *onvifMotionWatcher {
	return &onvifMotionWatcher{
		srv:                 srv,
		onvifURL:            onvifURL,
		holdTime:            holdTime,
		log:                 log,
		now:                 time.Now,
		newPullPoint:        newOnvifPullPoint,
		subscriptionTimeout: onvifSubscriptionTimeout,
		pullTimeout:         onvifPullTimeout,
		renewMargin:         onvifRenewMargin,
		messageLimit:        onvifMessageLimit,
		done:                make(chan struct{}),
	}
}

// startOnvifMotionWatcher creates and starts a new ONVIF motion watcher.
func startOnvifMotionWatcher(srv *hksv.Server, onvifURL string, holdTime time.Duration, log zerolog.Logger) *onvifMotionWatcher {
	w := newOnvifMotionWatcher(srv, onvifURL, holdTime, log)
	go w.run()
	return w
}

// stop shuts down the watcher goroutine.
func (w *onvifMotionWatcher) stop() {
	w.once.Do(func() { close(w.done) })
}

// run is the main loop: create subscription, poll, handle events, reconnect on failure.
func (w *onvifMotionWatcher) run() {
	w.log.Debug().Str("url", w.onvifURL).Dur("hold_time", w.holdTime).
		Msg("[homekit] onvif motion watcher starting")

	delay := onvifMinReconnectDelay

	for {
		select {
		case <-w.done:
			w.log.Debug().Msg("[homekit] onvif motion watcher stopped (before connect)")
			return
		default:
		}

		w.log.Debug().Str("url", w.onvifURL).Msg("[homekit] onvif motion connecting to camera")

		err := w.connectAndPoll()
		if err != nil {
			w.log.Warn().Err(err).Str("url", w.onvifURL).Msg("[homekit] onvif motion error")
		} else {
			delay = onvifMinReconnectDelay
		}

		select {
		case <-w.done:
			w.log.Debug().Msg("[homekit] onvif motion watcher stopped (after poll)")
			return
		default:
		}

		w.log.Debug().Dur("delay", delay).Msg("[homekit] onvif motion reconnecting")

		select {
		case <-time.After(delay):
		case <-w.done:
			w.log.Debug().Msg("[homekit] onvif motion watcher stopped (during backoff)")
			return
		}

		delay *= 2
		if delay > onvifMaxReconnectDelay {
			delay = onvifMaxReconnectDelay
		}
	}
}

// connectAndPoll creates a subscription and polls for events until an error occurs or stop is called.
func (w *onvifMotionWatcher) connectAndPoll() error {
	w.log.Trace().Str("url", w.onvifURL).Dur("timeout", w.subscriptionTimeout).
		Msg("[homekit] onvif motion: creating pull point subscription")

	sub, err := w.newPullPoint(w.onvifURL, w.subscriptionTimeout)
	if err != nil {
		w.log.Debug().Err(err).Str("url", w.onvifURL).
			Msg("[homekit] onvif motion: pull point creation failed")
		return err
	}

	w.log.Info().Str("url", w.onvifURL).Msg("[homekit] onvif motion subscription created")

	defer func() {
		w.log.Trace().Msg("[homekit] onvif motion: unsubscribing")
		_ = sub.Unsubscribe()
	}()

	// motionActive tracks whether we've reported motion=true to the HKSV server.
	// Hold timer ensures motion stays active for at least holdTime after last trigger,
	// regardless of whether the camera sends explicit "motion=false".
	// This matches the behavior of the built-in MotionDetector (30s hold time).
	motionActive := false
	var holdTimer *time.Timer
	defer func() {
		if holdTimer != nil {
			holdTimer.Stop()
		}
	}()

	renewInterval := w.subscriptionRenewInterval()
	renewAt := w.now().Add(renewInterval)

	w.log.Trace().Dur("renew_interval", renewInterval).
		Msg("[homekit] onvif motion: subscription renew scheduled")

	pollCount := 0

	for {
		select {
		case <-w.done:
			w.log.Debug().Int("polls", pollCount).
				Msg("[homekit] onvif motion: poll loop stopped")
			return nil
		default:
		}

		if !renewAt.After(w.now()) {
			w.log.Trace().Msg("[homekit] onvif motion: renewing subscription")
			if err := sub.Renew(w.subscriptionTimeout); err != nil {
				w.log.Warn().Err(err).Msg("[homekit] onvif motion: renew failed")
				return err
			}
			renewAt = w.now().Add(renewInterval)
			w.log.Trace().Msg("[homekit] onvif motion: subscription renewed")
		}

		pullTimeout := w.nextPullTimeout(renewAt)

		w.log.Trace().Dur("timeout", pullTimeout).Int("limit", w.messageLimit).
			Int("poll", pollCount+1).Msg("[homekit] onvif motion: pulling messages")

		b, err := sub.PullMessages(pullTimeout, w.messageLimit)
		if err != nil {
			w.log.Debug().Err(err).Int("polls", pollCount).
				Msg("[homekit] onvif motion: pull messages failed")
			return err
		}
		pollCount++

		w.log.Trace().Int("bytes", len(b)).Int("poll", pollCount).
			Msg("[homekit] onvif motion: pull response received")

		if l := w.log.Trace(); l.Enabled() {
			l.Str("body", string(b)).Msg("[homekit] onvif motion: raw response")
		}

		motion, found := onvif.ParseMotionEvents(b)

		w.log.Trace().Bool("found", found).Bool("motion", motion).
			Bool("active", motionActive).Msg("[homekit] onvif motion: parse result")

		if !found {
			w.log.Trace().Msg("[homekit] onvif motion: no motion events in response")
			continue
		}

		if motion {
			// Motion detected — activate and start/reset hold timer.
			if !motionActive {
				motionActive = true
				w.srv.SetMotionDetected(true)
				w.log.Debug().Msg("[homekit] onvif motion: detected")
			} else {
				w.log.Trace().Msg("[homekit] onvif motion: still active, resetting hold timer")
			}

			// Reset hold timer on every motion=true event.
			if holdTimer != nil {
				holdTimer.Stop()
			}
			holdTimer = time.AfterFunc(w.holdTime, func() {
				motionActive = false
				w.srv.SetMotionDetected(false)
				w.log.Debug().Msg("[homekit] onvif motion: hold expired")
			})
		} else {
			// Camera sent explicit motion=false.
			// Do NOT clear immediately — let the hold timer handle it.
			// This ensures motion stays active for at least holdTime,
			// giving the Home Hub enough time to open the DataStream.
			w.log.Debug().Dur("remaining_hold", w.holdTime).
				Bool("active", motionActive).
				Msg("[homekit] onvif motion: camera reported clear, waiting for hold timer")
		}
	}
}

func (w *onvifMotionWatcher) subscriptionRenewInterval() time.Duration {
	interval := w.subscriptionTimeout - w.renewMargin
	if interval <= 0 {
		interval = w.subscriptionTimeout / 2
	}
	if interval <= 0 {
		interval = time.Second
	}
	return interval
}

func (w *onvifMotionWatcher) nextPullTimeout(renewAt time.Time) time.Duration {
	timeout := w.pullTimeout
	if timeout <= 0 {
		timeout = time.Second
	}

	if untilRenew := renewAt.Sub(w.now()); untilRenew > 0 && untilRenew < timeout {
		timeout = untilRenew
	}

	if timeout <= 0 {
		timeout = time.Second
	}

	return timeout
}

func newOnvifPullPoint(rawURL string, timeout time.Duration) (onvifPullPoint, error) {
	client, err := onvif.NewClient(rawURL)
	if err != nil {
		return nil, err
	}
	return client.CreatePullPointSubscription(timeout)
}

// findOnvifURL looks for an onvif:// URL in stream sources.
func findOnvifURL(sources []string) string {
	for _, src := range sources {
		if strings.HasPrefix(src, "onvif://") || strings.HasPrefix(src, "onvif:") {
			return src
		}
	}
	return ""
}
