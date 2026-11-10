package webrtc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	pion "github.com/pion/webrtc/v4"
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

func TestCrealitySDP(t *testing.T) {
	sdp := `v=0
o=- 1495799811084970 1495799811084970 IN IP4 0.0.0.0
s=-
t=0 0
a=msid-semantic:WMS *
a=group:BUNDLE 0
m=video 9 UDP/TLS/RTP/SAVPF 96 98
a=rtcp-fb:98 nack
a=rtcp-fb:98 nack pli
a=fmtp:96 profile-level-id=42e01f;level-asymmetry-allowed=1
a=fmtp:98 profile-level-id=42e01f;packetization-mode=1;level-asymmetry-allowed=1
a=fmtp:98 x-google-max-bitrate=6000;x-google-min-bitrate=2000;x-google-start-bitrate=4000
a=rtpmap:96 H264/90000
a=rtpmap:98 H264/90000
a=ssrc:1 cname:pear
c=IN IP4 0.0.0.0
a=sendonly
a=mid:0
a=rtcp-mux
a=ice-ufrag:7AVa
a=ice-pwd:T+F/5y05Paw+mtG5Jrd8N3
a=ice-options:trickle
a=fingerprint:sha-256 A5:AB:C0:4E:29:5B:BD:3B:7D:88:24:6C:56:6B:2A:79:A3:76:99:35:57:75:AD:C8:5A:A6:34:20:88:1B:68:EF
a=setup:passive
a=candidate:1 1 UDP 2015363327 172.22.233.10 48929 typ host
a=candidate:2 1 TCP 1015021823 172.22.233.10 0 typ host tcptype active
a=candidate:3 1 TCP 1010827519 172.22.233.10 60677 typ host tcptype passive
`
	sdp, err := fixCrealitySDP(sdp, sdp)
	require.Nil(t, err)
	require.False(t, strings.Contains(sdp, "x-google-max-bitrate"))
}

// TestCrealitySDPUnknownPayloadType covers Creality printers (e.g. K1C) whose
// answer SDP doesn't match what they actually send: our own offer announces
// several H264 payload types (same as go2rtc's default codec table), but the
// device's answer only lists two of them, and on the wire it uses one that
// isn't in its own answer at all. A real browser still decodes the stream,
// because it falls back to matching the incoming payload type against its
// own offer. fixCrealitySDP should add the missing offered H264 payload
// types back to the answer so pion can do the same.
func TestCrealitySDPUnknownPayloadType(t *testing.T) {
	offer := `v=0
o=- 1 1 IN IP4 0.0.0.0
s=-
t=0 0
a=group:BUNDLE 0
m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 100
a=rtpmap:96 H264/90000
a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f
a=rtpmap:97 H264/90000
a=fmtp:97 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f
a=rtpmap:98 H264/90000
a=fmtp:98 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032
a=rtpmap:100 H265/90000
`
	answer := `v=0
o=- 1495799811084970 1495799811084970 IN IP4 0.0.0.0
s=-
t=0 0
a=msid-semantic:WMS *
a=group:BUNDLE 0
m=video 9 UDP/TLS/RTP/SAVPF 96 0
a=fmtp:96 profile-level-id=42e01f;level-asymmetry-allowed=1
a=fmtp:0 profile-level-id=42e01f;packetization-mode=1;level-asymmetry-allowed=1
a=fmtp:0 x-google-max-bitrate=6000;x-google-min-bitrate=2000;x-google-start-bitrate=4000
a=rtpmap:96 H264/90000
a=rtpmap:0 H264/90000
a=ssrc:1 cname:pear
c=IN IP4 0.0.0.0
a=sendonly
a=mid:0
a=rtcp-mux
a=setup:passive
`
	fixed, err := fixCrealitySDP(answer, offer)
	require.Nil(t, err)

	// codec 96 (the one we must skip) should be gone
	require.False(t, strings.Contains(fixed, "profile-level-id=42e01f;level-asymmetry-allowed=1\r"))
	// codec 0 (the one Creality's own logic keeps) is still there, cleaned up
	require.True(t, strings.Contains(fixed, "a=rtpmap:0 H264/90000"))
	// the payload types our own offer had for H264 (97, 98) must be added
	// back, because the device might stream using either of them instead of
	// what it declared in its answer
	require.True(t, strings.Contains(fixed, "a=rtpmap:97 H264/90000"))
	require.True(t, strings.Contains(fixed, "a=rtpmap:98 H264/90000"))
	// H265 (100) must not be pulled in, it's a different codec entirely
	require.False(t, strings.Contains(fixed, "H265"))
}
