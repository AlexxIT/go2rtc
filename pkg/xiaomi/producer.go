package xiaomi

import (
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/legacy"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

func Dial(rawURL string) (core.Producer, error) {
	// Format: xiaomi/miss
	if strings.Contains(rawURL, "vendor") {
		return miss.Dial(rawURL)
	}

	// Format: xiaomi/legacy
	return legacy.Dial(rawURL)
}

func IsLegacy(model string) bool {
	return legacy.Supported(model)
}
