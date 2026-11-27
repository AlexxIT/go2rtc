package webrtc

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// TestRenegotiationBackchannel - browser starts with recvonly audio and later
// enables two-way audio (recvonly => sendrecv) with renegotiation
func TestRenegotiationBackchannel(t *testing.T) {
	api, err := NewAPI()
	require.Nil(t, err)

	// go2rtc side
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)

	conn := NewConn(pc)
	conn.Mode = core.ModePassiveConsumer
	defer conn.Close()

	// browser side
	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)
	defer browser.Close()

	browserRecv := make(chan struct{}, 1)
	browser.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		for {
			if _, _, err := remote.ReadRTP(); err != nil {
				return
			}
			select {
			case browserRecv <- struct{}{}:
			default:
			}
		}
	})

	connected := make(chan struct{})
	browser.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
	})

	// 1. Initial negotiation: browser only wants to receive audio
	_, err = browser.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	)
	require.Nil(t, err)

	err = conn.SetOffer(createOffer(t, browser))
	require.Nil(t, err)

	// simulate streams.AddConsumer: camera audio => browser
	media := conn.getMedia("0", core.DirectionSendonly)
	require.NotNil(t, media)
	codec := getCodec(media, core.CodecOpus)
	require.NotNil(t, codec)

	camera := core.NewReceiver(
		&core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		&core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: 111},
	)
	err = conn.AddTrack(media, codec, camera)
	require.Nil(t, err)

	answer, err := conn.GetCompleteAnswer(nil, nil)
	require.Nil(t, err)
	require.Contains(t, answer, "a=sendonly")

	err = browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer})
	require.Nil(t, err)

	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("connection timeout")
	}

	// 2. Renegotiation: browser enables microphone on the same transceiver
	mic, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "browser",
	)
	require.Nil(t, err)

	_, err = browser.AddTrack(mic)
	require.Nil(t, err)
	require.Len(t, browser.GetTransceivers(), 1) // same transceiver reused

	reoffer := createOffer(t, browser)
	require.True(t, conn.IsReOffer(reoffer))

	medias, err := conn.SetReOffer(reoffer)
	require.Nil(t, err)
	require.Len(t, medias, 1)
	require.Equal(t, core.DirectionRecvonly, medias[0].Direction)
	require.Equal(t, "0", medias[0].ID)

	// simulate streams.AddConsumerMedias: browser audio => camera backchannel
	backCodec := getCodec(medias[0], core.CodecOpus)
	require.NotNil(t, backCodec)

	backchannel, err := conn.GetTrack(medias[0], backCodec)
	require.Nil(t, err)

	cameraRecv := make(chan struct{}, 1)
	sink := core.NewSender(medias[0], backCodec)
	sink.Handler = func(packet *rtp.Packet) {
		select {
		case cameraRecv <- struct{}{}:
		default:
		}
	}
	sink.WithParent(backchannel).Start()

	answer, err = conn.GetAnswer()
	require.Nil(t, err)
	require.Contains(t, answer, "a=sendrecv")

	err = browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer})
	require.Nil(t, err)

	// 3. Check media flows in both directions on the same connection
	var gotCamera, gotBrowser bool
	timeout := time.After(10 * time.Second)

	for seq := uint16(0); !gotCamera || !gotBrowser; seq++ {
		select {
		case <-cameraRecv:
			gotCamera = true
		case <-browserRecv:
			gotBrowser = true
		case <-timeout:
			t.Fatalf("media timeout: camera=%t browser=%t", gotCamera, gotBrowser)
		case <-time.After(20 * time.Millisecond):
		}

		packet := &rtp.Packet{
			Header: rtp.Header{
				Version: 2, PayloadType: 111, SequenceNumber: seq, Timestamp: uint32(seq) * 960, SSRC: 1234,
			},
			Payload: []byte{0xFC, 0xFF, 0xFE},
		}
		_ = mic.WriteRTP(packet)
		camera.WriteRTP(&rtp.Packet{Header: packet.Header, Payload: packet.Payload})
	}

	// 4. New offer from another browser is not a renegotiation
	other, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)
	defer other.Close()

	_, err = other.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.Nil(t, err)
	require.False(t, conn.IsReOffer(createOffer(t, other)))
}

func createOffer(t *testing.T, pc *webrtc.PeerConnection) string {
	offer, err := pc.CreateOffer(nil)
	require.Nil(t, err)

	gathered := webrtc.GatheringCompletePromise(pc)
	require.Nil(t, pc.SetLocalDescription(offer))
	<-gathered

	return pc.LocalDescription().SDP
}

func getCodec(media *core.Media, name string) *core.Codec {
	for _, codec := range media.Codecs {
		if strings.EqualFold(codec.Name, name) {
			return codec
		}
	}
	return nil
}
