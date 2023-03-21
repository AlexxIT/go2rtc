package webrtc

import (
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCandidates(t *testing.T) {
	conf := &ice.CandidateHostConfig{
		Network:   "udp",
		Address:   "192.168.1.123",
		Port:      8555,
		Component: ice.ComponentRTP,
	}
	cand, err := ice.NewCandidateHost(conf)
	require.Nil(t, err)
	assert.Equal(t, "candidate:"+cand.Marshal(), CandidateManualHostUDP(conf.Address, conf.Port))

	conf = &ice.CandidateHostConfig{
		Network:   "tcp",
		Address:   "192.168.1.123",
		Port:      8555,
		Component: ice.ComponentRTP,
		TCPType:   ice.TCPTypePassive,
	}
	cand, err = ice.NewCandidateHost(conf)
	require.Nil(t, err)
	assert.Equal(t, "candidate:"+cand.Marshal(), CandidateManualHostTCPPassive(conf.Address, conf.Port))
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
