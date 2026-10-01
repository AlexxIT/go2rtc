package webrtc

import (
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// browserOffer is the shape a browser sends for pc.addTrack(cameraTrack):
// every codec it supports, each with an rtx twin, AV1 first.
const browserOffer = `v=0
o=- 1 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
m=video 9 UDP/TLS/RTP/SAVPF 45 46 96 97 98 99 102 103
c=IN IP4 0.0.0.0
a=ice-ufrag:aaaa
a=ice-pwd:bbbbbbbbbbbbbbbbbbbbbbbb
a=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF
a=setup:actpass
a=mid:0
a=DIRECTION
a=rtcp-mux
a=rtpmap:45 AV1/90000
a=rtcp-fb:45 nack
a=rtcp-fb:45 nack pli
a=rtpmap:46 rtx/90000
a=fmtp:46 apt=45
a=rtpmap:96 VP8/90000
a=rtcp-fb:96 nack
a=rtpmap:97 rtx/90000
a=fmtp:97 apt=96
a=rtpmap:98 VP9/90000
a=rtpmap:99 rtx/90000
a=fmtp:99 apt=98
a=rtpmap:102 H264/90000
a=rtcp-fb:102 nack
a=rtcp-fb:102 nack pli
a=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f
a=rtpmap:103 rtx/90000
a=fmtp:103 apt=102
`

func answerForDirection(t *testing.T, direction string) string {
	t.Helper()

	api, err := NewAPI()
	require.Nil(t, err)

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)

	conn := NewConn(pc)
	offer := strings.ReplaceAll(strings.Replace(browserOffer, "DIRECTION", direction, 1), "\n", "\r\n")

	// a sendrecv m-line used to panic here, because the codec list still held rtx
	require.Nil(t, conn.SetOffer(offer))

	answer, err := conn.GetAnswer()
	require.Nil(t, err)

	return answer
}

func mediaLine(t *testing.T, sdp string) string {
	t.Helper()
	for _, line := range strings.Split(sdp, "\r\n") {
		if strings.HasPrefix(line, "m=video") {
			return line
		}
	}
	t.Fatal("no video media in answer")
	return ""
}

// TestPreferRecvCodecs - a remote publishes with the first codec of our answer,
// so AV1 must come last, without losing the payload types or the rtcp-fb of the
// offer.
func TestPreferRecvCodecs(t *testing.T) {
	for _, direction := range []string{"sendrecv", "sendonly"} {
		t.Run(direction, func(t *testing.T) {
			answer := answerForDirection(t, direction)

			// payload types of the offer, AV1 (45) behind H264 (102)
			require.Equal(t, "m=video 9 UDP/TLS/RTP/SAVPF 102 45", mediaLine(t, answer))

			// reordering must not drop the feedback attributes
			require.Contains(t, answer, "a=rtcp-fb:102 nack pli")
			require.Contains(t, answer, "a=rtcp-fb:45 nack pli")

			// and not rewrite the H264 profile
			require.Contains(t, answer, "profile-level-id=42001f")
		})
	}
}

// TestPreferRecvCodecsSendonly - we send on this transceiver, the remote only
// receives, so its codec order is none of our business.
func TestPreferRecvCodecsKeepsSendOrder(t *testing.T) {
	answer := answerForDirection(t, "recvonly")
	require.Equal(t, "m=video 9 UDP/TLS/RTP/SAVPF 45 102", mediaLine(t, answer))
}
