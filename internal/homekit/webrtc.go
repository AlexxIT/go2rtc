package homekit

import (
	"errors"

	iwebrtc "github.com/AlexxIT/go2rtc/internal/webrtc"
	pion "github.com/pion/webrtc/v4"
)

func newHomeKitPeerConnection() (*pion.PeerConnection, error) {
	if iwebrtc.PeerConnection == nil {
		return nil, errors.New("homekit: webrtc module not initialized")
	}
	// Use the server-side API so local ICE candidates match configured webrtc.listen
	return iwebrtc.PeerConnection(false)
}
