package debug

import (
	api "github.com/AlexxIT/go2rtc/internal/api/server"
)

func Init() {
	api.HandleFunc("api/stack", stackHandler)
}
