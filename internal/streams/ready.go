package streams

import "sync"

var (
	ready     = make(chan struct{})
	readyOnce sync.Once
)

func SetReady() {
	readyOnce.Do(func() {
		close(ready)
	})
}

func WaitReady() {
	<-ready
}

