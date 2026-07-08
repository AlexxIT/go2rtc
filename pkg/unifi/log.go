package unifi

import "github.com/rs/zerolog"

var log = zerolog.Nop()

func SetLogger(logger zerolog.Logger) {
	log = logger
}
