package webrtc

import (
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestName(t *testing.T) {
	i, _ := ice.NewCandidateHost(&ice.CandidateHostConfig{
		Network:   "tcp",
		Address:   "192.168.1.123",
		Port:      8555,
		Component: ice.ComponentRTP,
		TCPType:   ice.TCPTypePassive,
	})
	a := i.Marshal()
	println(a)
}

func TestPublicIP(t *testing.T) {
	ip, err := GetPublicIP()
	assert.Nil(t, err)
	assert.NotNil(t, ip)
	t.Logf("your public IP: %s", ip.String())
}

func TestMedia(t *testing.T) {
	codec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 96,
	}

	md := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media: "video", Protos: []string{"RTP", "AVP"},
		},
	}
	md.WithCodec(
		uint8(codec.PayloadType), codec.MimeType[6:], codec.ClockRate,
		codec.Channels, codec.SDPFmtpLine,
	)

	sd := &sdp.SessionDescription{
		MediaDescriptions: []*sdp.MediaDescription{md},
	}
	data, err := sd.Marshal()
	assert.Nil(t, err)
	assert.NotNil(t, data)
}
