package dtls

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pion "github.com/pion/dtls/v3"
)

func TestRDTAdapterCloseUnblocksReaderAndPreservesOtherChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	avData := make(chan []byte, 1)
	av := newChannelAdapter(ctx, iotcChannelMain, &net.UDPAddr{}, nil, avData)
	rdt := newChannelAdapter(ctx, iotcChannelRDT, &net.UDPAddr{}, nil, make(chan []byte))
	sibling := newChannelAdapter(ctx, iotcChannelRDT, &net.UDPAddr{}, nil, make(chan []byte))
	defer sibling.Close()
	read := make(chan error, 1)
	go func() {
		_, _, err := rdt.ReadFrom(make([]byte, 1))
		read <- err
	}()
	// Context cancellation is idempotent; repeated transport closes must be too.
	if err := rdt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rdt.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-read:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("closed RDT reader=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RDT transport Close left ReadFrom blocked")
	}
	if ctx.Err() != nil || av.ctx.Err() != nil || sibling.ctx.Err() != nil {
		t.Fatal("closing RDT cancelled the parent or an unrelated channel")
	}
	// AV Close deliberately retains the existing no-op contract. Data can still
	// arrive through its shared parent after both RDT and AV adapter Close calls.
	if err := av.Close(); err != nil {
		t.Fatal(err)
	}
	avData <- []byte{0x55}
	var payload [1]byte
	n, _, err := av.ReadFrom(payload[:])
	if n != 1 || err != nil || payload[0] != 0x55 || av.ctx != ctx {
		t.Fatalf("AV behavior changed: n=%d err=%v", n, err)
	}
}

func TestRDTDTLSCloseNotifiesPeerBeforeUnblockingReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientData, serverData := make(chan []byte, 64), make(chan []byte, 64)
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 32761}
	psk := bytes.Repeat([]byte{0x73}, 32)
	var clientAlert atomic.Bool
	clientWrite := func(packet []byte, channel uint8) error {
		if channel != iotcChannelRDT {
			return errors.New("unexpected channel")
		}
		// DTLS alert content type. With no handshake failure, this record is the
		// authenticated close_notify; its encrypted payload is not inspected.
		if len(packet) >= 13 && packet[0] == 21 {
			clientAlert.Store(true)
		}
		select {
		case serverData <- append([]byte(nil), packet...):
			return nil
		case <-ctx.Done():
			return net.ErrClosed
		}
	}
	serverWrite := func(packet []byte, _ uint8) error {
		select {
		case clientData <- append([]byte(nil), packet...):
			return nil
		case <-ctx.Done():
			return net.ErrClosed
		}
	}
	type connected struct {
		conn *pion.Conn
		err  error
	}
	serverReady := make(chan connected, 1)
	go func() {
		// The camera-facing client uses its custom ECDHE-PSK cipher suite;
		// emulate that peer here rather than AV backchannel's CBC server.
		adapter := newChannelAdapter(ctx, iotcChannelRDT, addr, serverWrite, serverData)
		conn, err := pion.Server(adapter, addr, buildDTLSConfig(psk, false))
		if err == nil {
			hsCtx, cancelHandshake := context.WithTimeout(ctx, 5*time.Second)
			err = conn.HandshakeContext(hsCtx)
			cancelHandshake()
		}
		if err != nil {
			_ = adapter.Close()
			if conn != nil {
				go conn.Close()
			}
		}
		serverReady <- connected{conn, err}
	}()
	client, err := NewDTLSClientWithIdentity(ctx, iotcChannelRDT, addr, clientWrite, clientData, psk, []byte("admin"))
	if err != nil {
		t.Fatal(err)
	}
	server := <-serverReady
	if server.err != nil {
		cancel()
		_ = client.Close()
		t.Fatal(server.err)
	}
	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Pion client Close=%v", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("Pion Close blocked on the channel adapter reader")
	}
	if !clientAlert.Load() {
		t.Fatal("Pion closed without emitting a DTLS close alert")
	}
	if ctx.Err() != nil {
		t.Fatal("RDT Close cancelled the parent connection")
	}
	go func() { closed <- server.conn.Close() }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("Pion server Close blocked on the channel adapter reader")
	}
}
