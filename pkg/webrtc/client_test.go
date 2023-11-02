package webrtc

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	api, err := NewAPI()
	require.Nil(t, err)

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)

	prod := NewConn(pc)

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionSendonly},
	}

	offer, err := prod.CreateOffer(medias)
	require.Nil(t, err)
	assert.NotEmpty(t, offer)

	require.Len(t, prod.pc.GetReceivers(), 2)
	require.Len(t, prod.pc.GetSenders(), 1)

	answer := `v=0
o=- 1934370540648269799 1678277622 IN IP4 0.0.0.0
s=-
t=0 0
a=fingerprint:sha-256 77:8C:9A:62:51:81:69:EA:4E:BE:93:6B:4E:DF:51:D2:2F:E3:DF:E7:F4:8A:18:1A:C0:74:FA:AE:B8:98:29:9B
a=extmap-allow-mixed
a=group:BUNDLE 0 1 2
m=video 9 UDP/TLS/RTP/SAVPF 97
c=IN IP4 0.0.0.0
a=setup:active
a=mid:0
a=ice-ufrag:xxx
a=ice-pwd:xxx
a=rtcp-mux
a=rtcp-rsize
a=rtpmap:97 H264/90000
a=fmtp:97 packetization-mode=1;profile-level-id=42e01f
a=extmap:1 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01
a=ssrc:2815449682 cname:go2rtc
a=ssrc:2815449682 msid:go2rtc video
a=ssrc:2815449682 mslabel:go2rtc
a=ssrc:2815449682 label:video
a=msid:go2rtc video
a=sendonly
m=audio 9 UDP/TLS/RTP/SAVPF 8
c=IN IP4 0.0.0.0
a=setup:active
a=mid:1
a=ice-ufrag:xxx
a=ice-pwd:xxx
a=rtcp-mux
a=rtcp-rsize
a=rtpmap:8 PCMA/8000
a=extmap:1 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01
a=ssrc:1392166302 cname:go2rtc
a=ssrc:1392166302 msid:go2rtc audio
a=ssrc:1392166302 mslabel:go2rtc
a=ssrc:1392166302 label:audio
a=msid:go2rtc audio
a=sendonly
m=audio 9 UDP/TLS/RTP/SAVPF 0
c=IN IP4 0.0.0.0
a=setup:active
a=mid:2
a=ice-ufrag:xxx
a=ice-pwd:xxx
a=rtcp-mux
a=rtcp-rsize
a=rtpmap:0 PCMU/8000
a=extmap:1 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01
a=recvonly
`

	err = prod.SetAnswer(answer)
	require.Nil(t, err)

	sender := prod.pc.GetSenders()[0]

	caps := webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypePCMU,
		ClockRate: 8000,
		Channels:  0,
	}
	track := sender.Track()
	track, err = webrtc.NewTrackLocalStaticRTP(caps, track.ID(), track.StreamID())
	require.Nil(t, err)

	err = sender.ReplaceTrack(track)
	require.Nil(t, err)
}

func TestUnmarshalICEServers(t *testing.T) {
	s := `[{"credential":"xxx","urls":"xxx","username":"xxx"},{"credential":null,"urls":"xxx","username":null}]`
	servers, err := UnmarshalICEServers([]byte(s))
	require.Nil(t, err)
	require.Len(t, servers, 2)
}
