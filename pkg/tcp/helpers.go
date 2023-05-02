package tcp

import (
	"net/http"
)

func RemoteAddr(r *http.Request) string {
	if remote := r.Header.Get("X-Forwarded-For"); remote != "" {
		return remote + ", " + r.RemoteAddr
	}
	return r.RemoteAddr
}
