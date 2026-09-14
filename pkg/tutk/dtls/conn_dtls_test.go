package dtls

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/stretchr/testify/require"
)

// dtlsPair connects a DTLS client and server through in-memory channels.
func dtlsPair(t *testing.T, ctx context.Context) (client, server *dtls.Conn) {
	psk := DerivePSK("test")
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	toClient := make(chan []byte, 1024)
	toServer := make(chan []byte, 1024)
	forward := func(ch chan []byte) func([]byte, uint8) error {
		return func(b []byte, _ uint8) error {
			ch <- append([]byte(nil), b...)
			return nil
		}
	}

	// Stand in for the camera: a server offering the client's custom suite.
	errs := make(chan error, 1)
	go func() {
		adapter := &channelAdapter{ctx: ctx, addr: addr, writeFn: forward(toClient), readChan: toServer}
		var err error
		if server, err = dtls.Server(adapter, addr, buildDTLSConfig(psk, false)); err == nil {
			err = server.HandshakeContext(ctx)
		}
		errs <- err
	}()

	client, err := NewDTLSClient(ctx, iotcChannelMain, addr, forward(toServer), toClient, psk)
	require.NoError(t, err)
	require.NoError(t, <-errs)
	return client, server
}

// Close clears clientConn while the ACK ticker started by AVClientStart is
// still running. Run with -race: the ticker must not read the field unlocked.
func TestCloseWhileAckTickerRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, server := dtlsPair(t, ctx)
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)

	connCtx, connCancel := context.WithCancel(ctx)
	c := &DTLSConn{
		conn:       udp,
		ctx:        connCtx,
		cancel:     connCancel,
		clientConn: client,
		rawCmd:     make(chan []byte, 1),
	}

	resp := make([]byte, 32)
	binary.LittleEndian.PutUint16(resp, magicAVLoginResp)
	c.rawCmd <- resp

	require.NoError(t, c.AVClientStart(time.Second))

	// Let the ACK ticker (100ms interval) fire at least once before closing.
	time.Sleep(250 * time.Millisecond)
	require.NoError(t, c.Close())
	require.Nil(t, c.getClientConn())
}
