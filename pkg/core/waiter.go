package core

import (
	"sync"
)

// Waiter support:
// - autotart on first Wait
// - block new waiters after last Done
// - safe Done after finish
type Waiter struct {
	sync.WaitGroup
	mu    sync.Mutex
	state int // state < 0 means finish
	err   error
}

func (w *Waiter) Add(delta int) {
	w.mu.Lock()
	if w.state >= 0 {
		w.state += delta
		w.WaitGroup.Add(delta)
	}
	w.mu.Unlock()
}

func (w *Waiter) Wait() error {
	w.mu.Lock()
	// first wait auto start waiter
	if w.state == 0 {
		w.state++
		w.WaitGroup.Add(1)
	}
	w.mu.Unlock()

	w.WaitGroup.Wait()

	return w.err
}

func (w *Waiter) Done(err error) {
	w.mu.Lock()

	// safe run Done only when have tasks
	if w.state > 0 {
		w.state--
		w.WaitGroup.Done()
	}

	// block waiter for any operations after last done
	if w.state == 0 {
		w.state = -1
		w.err = err
	}

	w.mu.Unlock()
}

func (w *Waiter) WaitChan() <-chan error {
	var ch chan error

	w.mu.Lock()

	if w.state >= 0 {
		ch = make(chan error)
		go func() {
			ch <- w.Wait()
		}()
	}

	w.mu.Unlock()

	return ch
}
