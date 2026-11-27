package webrtc

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// Exercise the actual connection lifecycle and Pion NACK responder over local ICE.
// The test receiver accepts a duplicate only so we can verify a requested repair
// without relying on random packet loss. Production replay protection is unchanged.
func TestConnectionRetransmitsNack(t *testing.T) {
	api, err := NewServerAPI("", "", &Filters{Loopback: true, Networks: []string{"udp4"}})
	require.NoError(t, err)
	pc, err := api.NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	conn := NewConn(pc)
	t.Cleanup(func() { _ = conn.Close() })

	settings := pion.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]pion.NetworkType{pion.NetworkTypeUDP4})
	settings.DisableSRTPReplayProtection(true)
	remoteAPI := pion.NewAPI(pion.WithSettingEngine(settings))
	remote, err := remoteAPI.NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	packets := make(chan *rtp.Packet, 8)
	remote.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		for {
			packet, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			select {
			case packets <- packet:
			default:
			}
		}
	})
	track := NewTrack("video")
	_, err = pc.AddTrack(track)
	require.NoError(t, err)
	_, err = pc.CreateDataChannel("test", nil)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	gathered := pion.GatheringCompletePromise(pc)
	require.NoError(t, pc.SetLocalDescription(offer))
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		t.Fatal("offer ICE timeout")
	}
	require.NoError(t, remote.SetRemoteDescription(*pc.LocalDescription()))
	answer, err := remote.CreateAnswer(nil)
	require.NoError(t, err)
	gathered = pion.GatheringCompletePromise(remote)
	require.NoError(t, remote.SetLocalDescription(answer))
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		t.Fatal("answer ICE timeout")
	}
	require.NoError(t, pc.SetRemoteDescription(*remote.LocalDescription()))
	require.Eventually(t, func() bool { return pc.ConnectionState() == pion.PeerConnectionStateConnected }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, track.WriteRTP(96, &rtp.Packet{
		Header: rtp.Header{Version: 2, Marker: true, Timestamp: 90000}, Payload: []byte{0x65, 0x88, 0x84},
	}))
	var original *rtp.Packet
	select {
	case original = <-packets:
	case <-time.After(2 * time.Second):
		t.Fatal("initial RTP missing")
	}
	require.NoError(t, remote.WriteRTCP([]rtcp.Packet{&rtcp.TransportLayerNack{
		SenderSSRC: 1, MediaSSRC: original.SSRC,
		Nacks: []rtcp.NackPair{{PacketID: original.SequenceNumber}},
	}}))
	select {
	case repaired := <-packets:
		require.Equal(t, original.SequenceNumber, repaired.SequenceNumber)
		require.Equal(t, original.Timestamp, repaired.Timestamp)
		require.Equal(t, original.Payload, repaired.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("go2rtc did not retransmit the packet requested by NACK")
	}
}
