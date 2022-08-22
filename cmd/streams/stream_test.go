package streams

import (
	"github.com/AlexxIT/go2rtc/pkg/fake"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// Google Chrome 104.0.5112.79
const chrome = `v=0
o=- 0 0 IN IP4 0.0.0.0
s=-
t=0 0
m=audio 9 UDP/TLS/RTP/SAVPF 111 63 103 104 9 0 8 110 112 113 126
a=sendrecv
a=rtpmap:111 opus/48000/2
a=rtpmap:63 red/48000/2
a=rtpmap:103 ISAC/16000
a=rtpmap:104 ISAC/32000
a=rtpmap:9 G722/8000
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:110 telephone-event/48000
a=rtpmap:112 telephone-event/32000
a=rtpmap:113 telephone-event/16000
a=rtpmap:126 telephone-event/8000
m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100 101 102 122 127 121 125 107 108 109 124 120 123 119 35 36 37 38 39 40 41 42 114 115 116 117 118 43
a=recvonly
a=rtpmap:96 VP8/90000
a=rtpmap:97 rtx/90000
a=rtpmap:98 VP9/90000
a=rtpmap:99 rtx/90000
a=rtpmap:100 VP9/90000
a=rtpmap:101 rtx/90000
a=rtpmap:102 VP9/90000
a=rtpmap:122 rtx/90000
a=rtpmap:127 H264/90000
a=fmtp:127 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f
a=rtpmap:121 rtx/90000
a=rtpmap:125 H264/90000
a=fmtp:125 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f
a=rtpmap:107 rtx/90000
a=rtpmap:108 H264/90000
a=fmtp:108 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f
a=rtpmap:109 rtx/90000
a=rtpmap:124 H264/90000
a=fmtp:124 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f
a=rtpmap:120 rtx/90000
a=rtpmap:123 H264/90000
a=fmtp:123 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=4d001f
a=rtpmap:119 rtx/90000
a=rtpmap:35 H264/90000
a=fmtp:35 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=4d001f
a=rtpmap:36 rtx/90000
a=rtpmap:37 H264/90000
a=fmtp:37 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=f4001f
a=rtpmap:38 rtx/90000
a=rtpmap:39 H264/90000
a=fmtp:39 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=f4001f
a=rtpmap:40 rtx/90000
a=rtpmap:41 AV1/90000
a=rtpmap:42 rtx/90000
a=rtpmap:114 H264/90000
a=fmtp:114 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f
a=rtpmap:115 rtx/90000
a=rtpmap:116 red/90000
a=rtpmap:117 rtx/90000
a=rtpmap:118 ulpfec/90000
a=rtpmap:43 flexfec-03/90000
`

const dahuaSimple = `v=0
o=- 0 0 IN IP4 0.0.0.0
s=-
t=0 0
m=video 0 RTP/AVP 96
a=control:trackID=0
a=rtpmap:96 H264/90000
a=fmtp:96 packetization-mode=1;profile-level-id=42401E;sprop-parameter-sets=Z0JAHqaAoD2QAA==,aM48gAA=
a=recvonly
m=audio 0 RTP/AVP 97
a=control:trackID=1
a=rtpmap:97 MPEG4-GENERIC/16000
a=fmtp:97 streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1408
a=recvonly
m=audio 0 RTP/AVP 8
a=control:trackID=5
a=rtpmap:8 PCMA/8000
a=sendonly
`

const ffmpegPCMU48000 = `v=0
o=- 0 0 IN IP4 127.0.0.1
s=-
t=0 0
m=audio 0 RTP/AVP 96
b=AS:384
a=rtpmap:96 PCMU/48000/1
a=control:streamid=0
`

func TestRouting(t *testing.T) {
	prod := &fake.Producer{}
	prod.Medias, _ = rtsp.UnmarshalSDP([]byte(dahuaSimple))
	assert.Len(t, prod.Medias, 3)

	HandleFunc("fake", func(url string) (streamer.Producer, error) {
		return prod, nil
	})

	cons := &fake.Consumer{}
	cons.Medias, _ = streamer.UnmarshalSDP([]byte(chrome))
	assert.Len(t, cons.Medias, 3)

	// setup stream with one producer
	stream := NewStream("fake:")

	// main check:
	err := stream.AddConsumer(cons)
	assert.Nil(t, err)

	assert.Len(t, prod.Tracks, 2)
	assert.Len(t, cons.Tracks, 2)

	time.Sleep(time.Second)

	assert.Greater(t, prod.SendPackets,0)
	assert.Greater(t, cons.RecvPackets,0)

	assert.Greater(t, prod.RecvPackets,0)
	assert.Greater(t, cons.SendPackets,0)
}
