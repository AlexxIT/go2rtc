package tutk

import (
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
)

func NewDtlsClient(c *Conn, channel uint8, psk []byte) (*dtls.Conn, error) {
	adapter := &ChannelAdapter{conn: c, channel: channel}
	return dtls.Client(adapter, c.addr, buildDtlsConfig(psk, false))
}

func NewDtlsServer(c *Conn, channel uint8, psk []byte) (*dtls.Conn, error) {
	adapter := &ChannelAdapter{conn: c, channel: channel}
	return dtls.Server(adapter, c.addr, buildDtlsConfig(psk, true))
}

func buildDtlsConfig(psk []byte, isServer bool) *dtls.Config {
	config := &dtls.Config{
		PSK: func(hint []byte) ([]byte, error) {
			return psk, nil
		},
		PSKIdentityHint:         []byte(PSKIdentity),
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
	conn    *Conn
	channel uint8

	mu           sync.Mutex
	readDeadline time.Time
}

func (a *ChannelAdapter) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	var buf chan []byte
	if a.channel == IOTCChannelMain {
		buf = a.conn.clientBuf
	} else {
		buf = a.conn.serverBuf
	}

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
		case data := <-buf:
			return copy(p, data), a.conn.addr, nil
		case <-timer.C:
			return 0, nil, &timeoutError{}
		case <-a.conn.ctx.Done():
			return 0, nil, net.ErrClosed
		}
	}

	select {
	case data := <-buf:
		return copy(p, data), a.conn.addr, nil
	case <-a.conn.ctx.Done():
		return 0, nil, net.ErrClosed
	}
}

func (a *ChannelAdapter) WriteTo(p []byte, _ net.Addr) (int, error) {
	if err := a.conn.WriteDTLS(p, a.channel); err != nil {
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
