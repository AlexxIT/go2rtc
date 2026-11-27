package tutk

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeMsg builds a decoded TUTK/0x19 message: header, size, type bytes at offset 8.
func fakeMsg(n int, typ string) []byte {
	b := make([]byte, n)
	copy(b, "\x04\x02\x19\x02")
	binary.LittleEndian.PutUint16(b[4:], uint16(n-16))
	copy(b[8:], typ)
	return b
}

// TestConnectRemote runs the Mi Home handshake against a fake P2P server and a fake camera on loopback.
func TestConnectRemote(t *testing.T) {
	remoteServers = nil // never talk to the real servers from a test

	listen := func() *net.UDPConn {
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	server, camera := listen(), listen()
	camAddr := camera.LocalAddr().(*net.UDPAddr)

	// serve answers the datagrams the client sends, like the P2P server and the camera do.
	serve := func(c *net.UDPConn, reply func(r []byte) []byte, got chan<- []byte) {
		buf := make([]byte, 1500)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			r := ReverseTransCodePartial(nil, buf[:n])
			if got != nil {
				got <- r
			}
			if b := reply(r); b != nil {
				_, _ = c.WriteToUDP(TransCodePartial(nil, b), from)
			}
		}
	}

	go serve(server, func(r []byte) []byte {
		switch {
		case r[8] == 0x03 && r[9] == 0x80: // 0x8003: who am I -> 0x8004 with the public address
			b := fakeMsg(24, "\x04\x80\x3f")
			copy(b[16:], "\x7f\x00\x00\x01\x00\x00\x00\x00")
			return b
		case r[8] == 0x03 && r[9] == 0x02: // 0x0302: where is the camera -> 0x0103
			b := fakeMsg(52, "\x01\x03\x43")
			binary.BigEndian.PutUint16(b[38:], uint16(camAddr.Port))
			copy(b[40:], camAddr.IP.To4())
			return b
		}
		return nil
	}, nil)

	stages := make(chan []byte, 16)
	go serve(camera, func(r []byte) []byte {
		switch r[8] {
		case 1: // first stage: answer 0x0401
			return fakeMsg(16, "\x01\x04\x33")
		case 2: // second stage: answer 0x0404
			return fakeMsg(16, "\x04\x04\x33")
		}
		return nil
	}, stages)

	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	c := &Conn{UDPConn: udp, addr: server.LocalAddr().(*net.UDPAddr)}
	t.Cleanup(func() { _ = c.Close() })

	done := make(chan error, 1)
	go func() { done <- c.connectRemote("TESTUID0123456789AB", GenSessionID()) }()

	select {
	case err = <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("handshake did not finish")
	}

	require.Equal(t, camAddr.Port, c.addr.Port, "traffic switches to the camera's address")
	require.Equal(t, []byte{0x19}, c.ver)

	// the closing stage 4 is written just before connectRemote returns, so it may still be in flight
	timeout := time.After(2 * time.Second)
	for {
		select {
		case r := <-stages:
			if r[8] == 4 {
				return
			}
		case <-timeout:
			t.Fatal("camera never got stage 4")
		}
	}
}
