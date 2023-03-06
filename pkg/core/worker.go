package core

import (
	"time"
)

type Worker struct {
	timer *time.Timer
	done  chan struct{}
}

// NewWorker run f after d
func NewWorker(d time.Duration, f func() time.Duration) *Worker {
	timer := time.NewTimer(d)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-timer.C:
				if d = f(); d > 0 {
					timer.Reset(d)
					continue
				}
			case <-done:
				timer.Stop()
			}
			break
		}
	}()

	return &Worker{timer: timer, done: done}
}

// Do - instant timer run
func (w *Worker) Do() {
	if w == nil {
		return
	}
	w.timer.Reset(0)
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}

	select {
	case w.done <- struct{}{}:
	default:
	}
}
