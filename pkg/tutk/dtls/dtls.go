package dtls

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
)

func NewDTLSClient(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte, psk []byte) (*dtls.Conn, error) {
	return dialDTLS(ctx, channel, addr, writeFn, readChan, psk, false)
}

// NewDTLSClientWithIdentity permits a separate RDT callback username without
// changing the AV constructor's existing AUTHPWD_admin identity.
func NewDTLSClientWithIdentity(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte, psk, identity []byte) (*dtls.Conn, error) {
	return dialDTLSIdentity(ctx, channel, addr, writeFn, readChan, psk, false, identity)
}

func NewDTLSServer(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte, psk []byte) (*dtls.Conn, error) {
	return dialDTLS(ctx, channel, addr, writeFn, readChan, psk, true)
}

func dialDTLS(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte, psk []byte, isServer bool) (*dtls.Conn, error) {
	return dialDTLSIdentity(ctx, channel, addr, writeFn, readChan, psk, isServer, []byte("AUTHPWD_admin"))
}

func dialDTLSIdentity(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte, psk []byte, isServer bool, identity []byte) (*dtls.Conn, error) {
	adapter := newChannelAdapter(ctx, channel, addr, writeFn, readChan)

	var conn *dtls.Conn
	var err error
	config := buildDTLSConfig(psk, isServer)
	config.PSKIdentityHint = append([]byte(nil), identity...)

	if isServer {
		conn, err = dtls.Server(adapter, addr, config)
	} else {
		conn, err = dtls.Client(adapter, addr, config)
	}
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}

	timeout := 5 * time.Second
	adapter.SetReadDeadline(time.Now().Add(timeout))
	hsCtx, cancel := context.WithTimeout(adapter.ctx, timeout)
	defer cancel()

	if err := conn.HandshakeContext(hsCtx); err != nil {
		// A failed RDT handshake must release its transport reader too. For AV
		// channels Close retains its existing no-op behavior.
		_ = adapter.Close()
		go conn.Close()
		return nil, err
	}

	adapter.SetReadDeadline(time.Time{})
	return conn, nil
}

func buildDTLSConfig(psk []byte, isServer bool) *dtls.Config {
	config := &dtls.Config{
		PSK: func(hint []byte) ([]byte, error) {
			return psk, nil
		},
		PSKIdentityHint:         []byte("AUTHPWD_admin"),
		InsecureSkipVerify:      true,
		InsecureSkipVerifyHello: true,
		MTU:                     1200,
		FlightInterval:          300 * time.Millisecond,
		ExtendedMasterSecret:    dtls.DisableExtendedMasterSecret,
	}

	if isServer {
		config.CipherSuites = []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_CBC_SHA256}
	} else {
		config.CustomCipherSuites = CustomCipherSuites
	}

	return config
}

type channelAdapter struct {
	ctx          context.Context
	closeCancel  context.CancelFunc
	channel      uint8
	writeFn      func([]byte, uint8) error
	readChan     chan []byte
	addr         net.Addr
	mu           sync.Mutex
	readDeadline time.Time
}

func newChannelAdapter(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte) *channelAdapter {
	a := &channelAdapter{ctx: ctx, channel: channel, addr: addr, writeFn: writeFn, readChan: readChan}
	if channel == iotcChannelRDT {
		// Pion sends close_notify before closing its transport and waiting for
		// the reader. Own an RDT-only cancellation so transport Close can wake
		// ReadFrom at that point, without prematurely cancelling close_notify
		// or affecting the parent AV connection and other channels.
		a.ctx, a.closeCancel = context.WithCancel(ctx)
	}
	return a
}

func (a *channelAdapter) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	a.mu.Lock()
	deadline := a.readDeadline
	a.mu.Unlock()

	if !deadline.IsZero() {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, nil, &timeoutError{}
		}

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case data := <-a.readChan:
			return copy(p, data), a.addr, nil
		case <-timer.C:
			return 0, nil, &timeoutError{}
		case <-a.ctx.Done():
			return 0, nil, net.ErrClosed
		}
	}

	select {
	case data := <-a.readChan:
		return copy(p, data), a.addr, nil
	case <-a.ctx.Done():
		return 0, nil, net.ErrClosed
	}
}

func (a *channelAdapter) WriteTo(p []byte, _ net.Addr) (int, error) {
	if err := a.writeFn(p, a.channel); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *channelAdapter) Close() error {
	if a.closeCancel != nil {
		a.closeCancel()
	}
	return nil
}
func (a *channelAdapter) LocalAddr() net.Addr { return &net.UDPAddr{} }
func (a *channelAdapter) SetDeadline(t time.Time) error {
	a.mu.Lock()
	a.readDeadline = t
	a.mu.Unlock()
	return nil
}
func (a *channelAdapter) SetReadDeadline(t time.Time) error {
	a.mu.Lock()
	a.readDeadline = t
	a.mu.Unlock()
	return nil
}
func (a *channelAdapter) SetWriteDeadline(time.Time) error { return nil }

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
