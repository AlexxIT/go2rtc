package webrtc

import (
	"errors"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
)

// sessionHandler opts future offers on this transport into a shared lifetime.
// Clients must wait for the acknowledgement before sending an offer.
func sessionHandler(tr *ws.Transport, msg *ws.Message) error {
	if msg.String() != "close-on-disconnect-v1" {
		return errors.New("unsupported WebRTC session lifetime")
	}
	tr.WithContext(func(ctx map[any]any) {
		ctx["webrtc_lifetime"] = true
	})
	tr.Write(&ws.Message{Type: "webrtc/session", Value: "close-on-disconnect-v1"})
	return nil
}
