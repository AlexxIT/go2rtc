package isapi

import "github.com/rs/zerolog"

// Log is set from internal/isapi.Init. Defaults to no-op so Dial stays usable in tests.
var Log = zerolog.Nop()

func SetLogger(l zerolog.Logger) {
	Log = l
}
