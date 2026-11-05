package srtp

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func newTestEndpoint(port int, ssrc uint32) *Endpoint {
	return &Endpoint{
		Addr:       "127.0.0.1",
		Port:       uint16(port),
		MasterKey:  []byte(core.RandString(16, 0)),
		MasterSalt: []byte(core.RandString(14, 0)),
		SSRC:       ssrc,
	}
}

func TestSessionNotAdded(t *testing.T) {
	session := &Session{Local: newTestEndpoint(0, 1), Remote: newTestEndpoint(9, 2)}
	require.NoError(t, session.init()) // init OK, but no connection (not added to a server)

	n, err := session.WriteRTP(&rtp.Packet{Payload: []byte{1}})
	require.NoError(t, err)
	require.Zero(t, n)

	n, err = session.WriteRTCP(&rtcp.ReceiverReport{SSRC: 1})
	require.NoError(t, err)
	require.Zero(t, n)
}

// WriteRTP (sender goroutine) and WriteRTCP (server goroutine, receiver reports)
// share the same SRTP context, run with -race
func TestSessionConcurrentWrite(t *testing.T) {
	remote, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer remote.Close()

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := remote.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	server := NewServer("127.0.0.1:0")
	session := &Session{
		Local:        newTestEndpoint(0, 1),
		Remote:       newTestEndpoint(remote.LocalAddr().(*net.UDPAddr).Port, 2),
		PayloadType:  110,
		RTCPInterval: time.Millisecond, // sender reports from WriteRTP
	}
	server.AddSession(session)
	defer server.DelSession(session)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := 0; seq < 300; seq++ {
				_, _ = session.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(seq)}, Payload: []byte{1, 2, 3}})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			_, _ = session.WriteRTCP(&rtcp.ReceiverReport{SSRC: 1})
		}
	}()
	wg.Wait()
}
