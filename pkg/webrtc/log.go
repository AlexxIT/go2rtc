package webrtc

import "github.com/rs/zerolog"

var log = zerolog.Nop()

// SetLogger sets the logger for debug messages of this package.
func SetLogger(logger zerolog.Logger) {
	log = logger
}
