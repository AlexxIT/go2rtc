package dtls

import (
	"context"
	"errors"
	"fmt"
	"net"
)

const iotcChannelRDT = 10

// ProbeRDTHandshake tests and closes the separate channel-10 secure transport. It sends
// no RDT HELLO, file operation, application data, or AV control command.
// Static provenance: Wyze's RDT_Client_Create uses IOTC_sCHL on channel 10;
// the native callback returns all 32 bytes of SHA256(ENR) and username admin.
// This uses that raw callback username as a bounded identity test. Whether
// IOTC_sCHL applies an additional prefix is not established by the RDT callback.
// NewDTLSClient enforces its existing explicit five-second handshake deadline.
func (c *DTLSConn) ProbeRDTHandshake(psk []byte) error {
	if c == nil {
		return errors.New("authenticated transport required")
	}
	c.rdtMu.Lock()
	defer c.rdtMu.Unlock()
	defer c.closeRDTHandshakeLocked()
	return c.openRDTChannelLocked(psk)
}

// Caller holds rdtMu for the channel's entire lifetime, including teardown.
func (c *DTLSConn) openRDTChannelLocked(psk []byte) error {
	if c == nil || c.ctx == nil || c.conn == nil || len(psk) != 32 {
		return errors.New("RDT handshake requires an authenticated transport and 32-byte PSK")
	}
	if c.ctx.Err() != nil {
		return net.ErrClosed
	}
	// Reopening channel 10 on the same SID is not interoperable with the tested
	// firmware. Fail explicitly rather than returning a cached or stale channel.
	if c.rdtUsed {
		return errors.New("RDT requires a fresh authenticated session")
	}
	c.rdtUsed = true
	ctx, cancel := context.WithCancel(c.ctx)
	buffer := make(chan []byte, 64)
	c.rdtReceived.Store(0)
	// Publish before NewDTLSClient can send its first ClientHello. The UDP
	// reader can immediately route a camera response without racing startup.
	c.mu.Lock()
	c.rdtBuf = buffer
	c.rdtCancel = cancel
	c.mu.Unlock()
	conn, err := NewDTLSClientWithIdentity(ctx, iotcChannelRDT, c.remoteAddrSnapshot(), c.WriteDTLS, buffer, append([]byte(nil), psk...), []byte("admin"))
	if err != nil {
		cancel()
		c.mu.Lock()
		c.rdtBuf = nil
		c.rdtCancel = nil
		c.mu.Unlock()
		transport := "iotc"
		if c.isCC51 { transport = "cc51" }
		return fmt.Errorf("RDT transport=%s: %w", transport, safeRDTHandshakeFailure(err, c.rdtReceived.Load()))
	}
	c.mu.Lock()
	c.rdtConn = conn
	c.mu.Unlock()
	return nil
}

func (c *DTLSConn) remoteAddrSnapshot() *net.UDPAddr {
	c.addrMu.RLock()
	defer c.addrMu.RUnlock()
	if c.addr == nil {
		return nil
	}
	return &net.UDPAddr{IP: append(net.IP(nil), c.addr.IP...), Port: c.addr.Port, Zone: c.addr.Zone}
}

// routeDTLSRecord is shared by old IOTC and CC51 packet readers. It preserves
// channel isolation and copies receive data before the UDP buffer is reused.
func (c *DTLSConn) routeDTLSRecord(channel byte, data []byte) {
	var buffer chan []byte
	switch channel {
	case iotcChannelMain:
		buffer = c.clientBuf
	case iotcChannelBack:
		buffer = c.serverBuf
	case iotcChannelRDT:
		c.mu.RLock()
		buffer = c.rdtBuf
		c.mu.RUnlock()
		if buffer != nil {
			c.rdtReceived.Add(1)
		}
	}
	if buffer != nil {
		c.queue(buffer, data)
	}
}

func (c *DTLSConn) closeRDTHandshake() {
	c.rdtMu.Lock()
	defer c.rdtMu.Unlock()
	c.closeRDTHandshakeLocked()
}

func (c *DTLSConn) closeRDTHandshakeLocked() {
	c.mu.Lock()
	conn, cancel := c.rdtConn, c.rdtCancel
	c.rdtConn, c.rdtCancel, c.rdtBuf = nil, nil, nil
	c.mu.Unlock()
	if conn != nil {
		// Let Pion send close_notify before its adapter closes and cancels
		// the RDT-only reader. Cancelling the parent first races that alert.
		_ = conn.Close()
	}
	if cancel != nil {
		cancel()
	}
}

// Never return an upstream error string; only fixed classes and counts are safe.
func safeRDTHandshakeFailure(err error, received uint64) error {
	classification := "protocol_error"
	alertLevel, alertDescription := -1, -1
	var timeout net.Error
	// Pion's unexported alertError embeds *alert.Alert, promoting Marshal.
	// Its inspected Marshal implementation returns only [level, description].
	var peerAlert interface {
		IsFatalOrCloseNotify() bool
		Marshal() ([]byte, error)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		classification = "timeout"
	case errors.Is(err, context.Canceled), errors.Is(err, net.ErrClosed):
		classification = "cancelled_or_closed"
	case errors.As(err, &peerAlert):
		classification = "peer_alert"
		if wire, marshalErr := peerAlert.Marshal(); marshalErr == nil && len(wire) == 2 {
			alertLevel, alertDescription = int(wire[0]), int(wire[1])
		}
	case errors.As(err, &timeout) && timeout.Timeout():
		classification = "timeout"
	}
	return fmt.Errorf("RDT channel-10 DTLS failed (class=%s, incoming_datagrams=%d, alert_level=%d, alert_description=%d)", classification, received, alertLevel, alertDescription)
}
