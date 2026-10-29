package server

import (
	"net/http"
	"slices"
)

// HandleFunc handle pattern with relative path:
// - "api/streams" => "{basepath}/api/streams"
// - "/streams"    => "/streams"
func HandleFunc(pattern string, handler http.HandlerFunc) {
	if len(pattern) == 0 || pattern[0] != '/' {
		pattern = basePath + "/" + pattern
	}
	if allowPaths != nil && !slices.Contains(allowPaths, pattern) {
		log.Trace().Str("path", pattern).Msg("[api] ignore path not in allow_paths")
		return
	}
	log.Trace().Str("path", pattern).Msg("[api] register path")
	http.HandleFunc(pattern, handler)
}
