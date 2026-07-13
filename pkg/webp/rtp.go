package webp

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
)

// RTPDepay depayloads RTP/JPEG packets and converts the resulting JPEG frame to WebP.
func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	return mjpeg.RTPDepay(JPEGToWebP(handler))
}
