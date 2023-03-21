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
}

func (w *Waiter) Add(delta int) {
	w.mu.Lock()
	if w.state >= 0 {
		w.state += delta
		w.WaitGroup.Add(delta)
	}
	w.mu.Unlock()
}

func (w *Waiter) Wait() {
	w.mu.Lock()
	// first wait auto start waiter
	if w.state == 0 {
		w.state++
		w.WaitGroup.Add(1)
	}
	w.mu.Unlock()

	w.WaitGroup.Wait()
}

func (w *Waiter) Done() {
	w.mu.Lock()

	// safe run Done only when have tasks
	if w.state > 0 {
		w.state--
		w.WaitGroup.Done()
	}

	// block waiter for any operations after last done
	if w.state == 0 {
		w.state = -1
	}

	w.mu.Unlock()
}

func (w *Waiter) WaitChan() <-chan struct{} {
	var ch chan struct{}

	w.mu.Lock()

	if w.state >= 0 {
		ch = make(chan struct{})
		go func() {
			w.Wait()
			ch <- struct{}{}
		}()
	}

	w.mu.Unlock()

	return ch
}
