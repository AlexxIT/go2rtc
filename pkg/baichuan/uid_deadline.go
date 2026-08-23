package baichuan

import (
	"sync"
	"time"
)

type uidDeadline struct {
	mu      sync.Mutex
	value   time.Time
	changed chan struct{}
	timer   *time.Timer
}

func (d *uidDeadline) set(value time.Time) {
	d.mu.Lock()
	if !d.timer.Stop() {
		select {
		case <-d.timer.C:
		default:
		}
	}
	close(d.changed)
	d.value = value
	d.changed = make(chan struct{})
	if !value.IsZero() {
		duration := time.Until(value)
		if duration < 0 {
			duration = 0
		}
		d.timer.Reset(duration)
	}
	d.mu.Unlock()
}

func (d *uidDeadline) init() {
	d.changed = make(chan struct{})
	d.timer = time.NewTimer(time.Hour)
	d.timer.Stop()
}

func (d *uidDeadline) snapshot() (time.Time, <-chan struct{}, <-chan time.Time) {
	d.mu.Lock()
	value, changed := d.value, d.changed
	var timeout <-chan time.Time
	if !value.IsZero() {
		timeout = d.timer.C
	}
	d.mu.Unlock()
	return value, changed, timeout
}
