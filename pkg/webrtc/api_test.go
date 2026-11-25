package webrtc

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func startTestSTUNServer(t *testing.T) (string, *atomic.Int32) {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	requests := &atomic.Int32{}
	go func() {
		buf := make([]byte, 1500)
		for {
			n, raddr, readErr := conn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			var req stun.Message
			req.Raw = buf[:n]
			if req.Decode() != nil || req.Type != stun.BindingRequest {
				continue
			}
			requests.Add(1)

			res, buildErr := stun.Build(
				&req,
				stun.BindingSuccess,
				&stun.XORMappedAddress{IP: net.IPv4(203, 0, 113, 1), Port: raddr.Port},
			)
			if buildErr == nil {
				_, _ = conn.WriteToUDP(res.Raw, raddr)
			}
		}
	}()

	return "stun:" + conn.LocalAddr().String(), requests
}

func startRejectingTURNServerTCP(t *testing.T) (string, *atomic.Int32) {
	t.Helper()

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	dials := &atomic.Int32{}
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			dials.Add(1)
			go func(c net.Conn) {
				defer c.Close()
				var req stun.Message
				if _, readErr := req.ReadFrom(c); readErr != nil {
					return
				}
				res, buildErr := stun.Build(
					&req,
					stun.NewType(req.Type.Method, stun.ClassErrorResponse),
					stun.CodeForbidden,
				)
				if buildErr == nil {
					_, _ = c.Write(res.Raw)
				}
			}(conn)
		}
	}()

	return "turn:" + ln.Addr().String() + "?transport=tcp", dials
}

func gatherCandidates(t *testing.T, api *webrtc.API, servers []webrtc.ICEServer) []*webrtc.ICECandidate {
	t.Helper()

	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: servers})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })

	var candidates []*webrtc.ICECandidate
	done := make(chan struct{})
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			candidates = append(candidates, c)
		} else {
			close(done)
		}
	})

	_, err = pc.CreateDataChannel("test", nil)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, pc.SetLocalDescription(offer))

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ICE gathering timed out")
	}

	return candidates
}

// Regression test for pion/ice v4.4+ STUN/TURN socket fan-out when Filters.IPs
// is unset: NewServerAPI must leave SettingEngine.SetIPFilter nil by default so
// STUN and TURN gathering bind a single wildcard socket instead of fanning out
// across every local interface IP.
func TestDefaultIPFilterDoesNotFanOutSTUNOrTURN(t *testing.T) {
	t.Run("default filters use wildcard STUN socket", func(t *testing.T) {
		stunURL, requests := startTestSTUNServer(t)

		api, err := NewServerAPI("", "", &Filters{Loopback: true, Networks: []string{"udp4"}})
		require.NoError(t, err)

		candidates := gatherCandidates(t, api, []webrtc.ICEServer{{URLs: []string{stunURL}}})

		var srflx []*webrtc.ICECandidate
		for _, c := range candidates {
			if c.Typ == webrtc.ICECandidateTypeSrflx {
				srflx = append(srflx, c)
			}
		}
		require.Len(t, srflx, 1)
		require.Equal(t, "0.0.0.0", srflx[0].RelatedAddress)
		require.EqualValues(t, 1, requests.Load())
	})

	t.Run("default filters dial TCP TURN once", func(t *testing.T) {
		turnURL, dials := startRejectingTURNServerTCP(t)

		api, err := NewServerAPI("", "", &Filters{Loopback: true, Networks: []string{"udp4", "tcp4"}})
		require.NoError(t, err)

		_ = gatherCandidates(t, api, []webrtc.ICEServer{{
			URLs:       []string{turnURL},
			Username:   "user",
			Credential: "pass",
		}})
		require.EqualValues(t, 1, dials.Load())
	})

	t.Run("explicit Filters.IPs binds configured IP", func(t *testing.T) {
		stunURL, _ := startTestSTUNServer(t)

		api, err := NewServerAPI("", "", &Filters{
			Loopback: true,
			IPs:      []string{"127.0.0.1"},
			Networks: []string{"udp4"},
		})
		require.NoError(t, err)

		candidates := gatherCandidates(t, api, []webrtc.ICEServer{{URLs: []string{stunURL}}})

		var srflx []*webrtc.ICECandidate
		for _, c := range candidates {
			switch c.Typ {
			case webrtc.ICECandidateTypeHost:
				require.Equal(t, "127.0.0.1", c.Address)
			case webrtc.ICECandidateTypeSrflx:
				srflx = append(srflx, c)
			}
		}
		require.Len(t, srflx, 1)
		require.Equal(t, "127.0.0.1", srflx[0].RelatedAddress)
	})
}
