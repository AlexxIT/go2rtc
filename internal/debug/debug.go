package debug

import (
	"github.com/AlexxIT/go2rtc/internal/api"
)

func Init() {
	api.HandleFunc("api/stack", stackHandler)
}
