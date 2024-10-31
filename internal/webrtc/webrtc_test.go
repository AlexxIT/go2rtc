package webrtc

import (
	"encoding/json"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	pion "github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestWebRTCAPIv1(t *testing.T) {
	raw := `{"type":"webrtc/offer","value":"v=0\n..."}`
	msg := new(ws.Message)
	err := json.Unmarshal([]byte(raw), msg)
	require.Nil(t, err)

	require.Equal(t, "v=0\n...", msg.String())
}

func TestWebRTCAPIv2(t *testing.T) {
	raw := `{"type":"webrtc","value":{"type":"offer","sdp":"v=0\n...","ice_servers":[{"urls":["stun:stun.l.google.com:19302"]}]}}`
	msg := new(ws.Message)
	err := json.Unmarshal([]byte(raw), msg)
	require.Nil(t, err)

	var offer struct {
		Type       string           `json:"type"`
		SDP        string           `json:"sdp"`
		ICEServers []pion.ICEServer `json:"ice_servers"`
	}
	err = msg.Unmarshal(&offer)
	require.Nil(t, err)

	require.Equal(t, "offer", offer.Type)
	require.Equal(t, "v=0\n...", offer.SDP)
	require.Equal(t, "stun:stun.l.google.com:19302", offer.ICEServers[0].URLs[0])
}
