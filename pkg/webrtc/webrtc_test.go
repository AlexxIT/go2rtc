package webrtc

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestAlexa(t *testing.T) {
	// from https://github.com/AlexxIT/go2rtc/issues/825
	offer := `v=0
o=- 3911343731 3911343731 IN IP4 0.0.0.0
s=a 2 z
c=IN IP4 0.0.0.0
t=0 0
a=group:BUNDLE audio0 video0
m=audio 1 UDP/TLS/RTP/SAVPF 96 0 8
a=candidate:1 1 UDP 2013266431 52.90.193.210 60128 typ host
a=candidate:2 1 TCP 1015021823 52.90.193.210 9 typ host tcptype active
a=candidate:3 1 TCP 1010827519 52.90.193.210 45962 typ host tcptype passive
a=candidate:1 2 UDP 2013266430 52.90.193.210 46109 typ host
a=candidate:2 2 TCP 1015021822 52.90.193.210 9 typ host tcptype active
a=candidate:3 2 TCP 1010827518 52.90.193.210 53795 typ host tcptype passive
a=setup:actpass
a=extmap:3 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time
a=rtpmap:96 opus/48000/2
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtcp:9 IN IP4 0.0.0.0
a=rtcp-mux
a=sendrecv
a=mid:audio0
a=ssrc:3573704076 cname:user3856789923@host-9dd1dd33
a=ice-ufrag:gxfV
a=ice-pwd:KepKrlQ1+LD+RGTAFaqVck
a=fingerprint:sha-256 A2:93:53:50:E4:2F:C5:4E:DF:7C:70:99:5A:A7:39:50:1A:63:E5:B2:CA:73:70:7A:C5:F4:01:BF:BD:99:57:FC
m=video 1 UDP/TLS/RTP/SAVPF 99
a=candidate:1 1 UDP 2013266431 52.90.193.210 60128 typ host
a=candidate:1 2 UDP 2013266430 52.90.193.210 46109 typ host
a=candidate:2 1 TCP 1015021823 52.90.193.210 9 typ host tcptype active
a=candidate:3 1 TCP 1010827519 52.90.193.210 45962 typ host tcptype passive
a=candidate:3 2 TCP 1010827518 52.90.193.210 53795 typ host tcptype passive
a=candidate:2 2 TCP 1015021822 52.90.193.210 9 typ host tcptype active
b=AS:2500
a=setup:actpass
a=extmap:3 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time
a=rtpmap:99 H264/90000
a=rtcp:9 IN IP4 0.0.0.0
a=rtcp-mux
a=sendrecv
a=mid:video0
a=rtcp-fb:99 nack
a=rtcp-fb:99 nack pli
a=rtcp-fb:99 ccm fir
a=ssrc:3778078295 cname:user3856789923@host-9dd1dd33
a=ice-ufrag:gxfV
a=ice-pwd:KepKrlQ1+LD+RGTAFaqVck
a=fingerprint:sha-256 A2:93:53:50:E4:2F:C5:4E:DF:7C:70:99:5A:A7:39:50:1A:63:E5:B2:CA:73:70:7A:C5:F4:01:BF:BD:99:57:FC
`

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)

	conn := NewConn(pc)
	err = conn.SetOffer(offer)
	require.Nil(t, err)

	_, err = conn.GetAnswer()
	require.Nil(t, err)
}

func TestConnStateClose(t *testing.T) {
	newConn := func(t *testing.T) (*Conn, <-chan error) {
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		require.Nil(t, err)

		conn := NewConn(pc)

		// Start blocks until connection closed
		started := make(chan error, 1)
		go func() { started <- conn.Start() }()

		return conn, started
	}

	closed := func(started <-chan error) bool {
		select {
		case <-started:
			return true
		case <-time.After(100 * time.Millisecond):
			return false
		}
	}

	t.Run("default close on disconnected", func(t *testing.T) {
		conn, started := newConn(t)
		conn.onConnectionStateChange(webrtc.PeerConnectionStateDisconnected)
		require.True(t, closed(started))
		require.Equal(t, webrtc.PeerConnectionStateClosed, conn.pc.ConnectionState())
	})

	t.Run("ignore disconnected wait failed", func(t *testing.T) {
		conn, started := newConn(t)
		conn.IgnoreDisconnected = true

		var states []webrtc.PeerConnectionState
		conn.Listen(func(msg any) {
			if state, ok := msg.(webrtc.PeerConnectionState); ok {
				states = append(states, state)
			}
		})

		conn.onConnectionStateChange(webrtc.PeerConnectionStateDisconnected)
		require.False(t, closed(started))
		require.NotEqual(t, webrtc.PeerConnectionStateClosed, conn.pc.ConnectionState())

		// listeners still get the state
		require.Equal(t, []webrtc.PeerConnectionState{webrtc.PeerConnectionStateDisconnected}, states)

		// connection can recover
		conn.onConnectionStateChange(webrtc.PeerConnectionStateConnected)
		require.False(t, closed(started))

		conn.onConnectionStateChange(webrtc.PeerConnectionStateFailed)
		require.True(t, closed(started))
		require.Equal(t, webrtc.PeerConnectionStateClosed, conn.pc.ConnectionState())
	})

	t.Run("ignore disconnected close on closed", func(t *testing.T) {
		conn, started := newConn(t)
		conn.IgnoreDisconnected = true
		conn.onConnectionStateChange(webrtc.PeerConnectionStateClosed)
		require.True(t, closed(started))
	})
}
