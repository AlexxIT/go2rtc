package api

import (
	"net/http"
	"sync"
)

var readyCh = make(chan struct{})
var readyOnce sync.Once

func SetReady() {
	readyOnce.Do(func() { close(readyCh) })
}

func middlewareReady(path string, ready <-chan struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			select {
			case <-ready:
			case <-r.Context().Done():
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
