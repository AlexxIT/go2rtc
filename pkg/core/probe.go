package core

import "time"

type Probe struct {
	deadline time.Time
	items    map[any]struct{}
}

func NewProbe(enable bool) *Probe {
	if enable {
		return &Probe{
			deadline: time.Now().Add(time.Second * 3),
			items:    map[any]struct{}{},
		}
	} else {
		return nil
	}
}

// Active return true if probe enabled and not finish
func (p *Probe) Active() bool {
	return len(p.items) < 2 && time.Now().Before(p.deadline)
}

// Append safe to run if Probe is nil
func (p *Probe) Append(v any) {
	if p != nil {
		p.items[v] = struct{}{}
	}
}
