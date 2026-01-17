package tutk

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
)

type DTLSConfig struct {
	PSK      []byte
	Identity string
	IsServer bool
}

func NewDTLSClient(adapter net.PacketConn, addr net.Addr, psk []byte) (*dtls.Conn, error) {
	return dtls.Client(adapter, addr, buildDTLSConfig(psk, false))
}

func NewDTLSServer(adapter net.PacketConn, addr net.Addr, psk []byte) (*dtls.Conn, error) {
	return dtls.Server(adapter, addr, buildDTLSConfig(psk, true))
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

type ChannelAdapter struct {
	ctx          context.Context
	channel      uint8
	writeFn      func([]byte, uint8) error
	readChan     chan []byte
	addr         net.Addr
	mu           sync.Mutex
	readDeadline time.Time
}

func NewChannelAdapter(ctx context.Context, channel uint8, addr net.Addr, writeFn func([]byte, uint8) error, readChan chan []byte) *ChannelAdapter {
	return &ChannelAdapter{
		ctx:      ctx,
		channel:  channel,
		addr:     addr,
		writeFn:  writeFn,
		readChan: readChan,
	}
}

func (a *ChannelAdapter) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
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

func (a *ChannelAdapter) WriteTo(p []byte, _ net.Addr) (int, error) {
	if err := a.writeFn(p, a.channel); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *ChannelAdapter) Close() error        { return nil }
func (a *ChannelAdapter) LocalAddr() net.Addr { return &net.UDPAddr{} }
func (a *ChannelAdapter) SetDeadline(t time.Time) error {
	a.mu.Lock()
	a.readDeadline = t
	a.mu.Unlock()
	return nil
}
func (a *ChannelAdapter) SetReadDeadline(t time.Time) error {
	a.mu.Lock()
	a.readDeadline = t
	a.mu.Unlock()
	return nil
}
func (a *ChannelAdapter) SetWriteDeadline(time.Time) error { return nil }

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
