package nest

import "github.com/rs/zerolog"

// Log is the package logger. It is a no-op by default and is set by
// internal/nest to the application "nest" logger.
//
// Never log the nest URL or any token: the URL carries client_secret and
// refresh_token as query parameters.
var Log = zerolog.Nop()
