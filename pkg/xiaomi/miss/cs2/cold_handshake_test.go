package cs2

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// A camera may become ready after initial discovery packets are lost. Recovery
// should be prompt while connecting, and stop sending when discovery finishes.
func TestColdDiscoveryRecoversLossAndStops(t *testing.T) {
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	local, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	c := &udpConn{UDPConn: local, addr: peer.LocalAddr().(*net.UDPAddr)}
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	var received atomic.Int32
	go func() {
		buf := make([]byte, 64)
		for {
			_, addr, err := peer.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if received.Add(1) == 2 {
				reply := make([]byte, 16)
				reply[0], reply[1] = magic, msgPunchPkt
				_, _ = peer.WriteToUDP(reply, addr)
			}
		}
	}()
	start := time.Now()
	_, err = c.WriteUntil([]byte{magic, msgLanSearch, 0, 0}, func(b []byte) bool { return b[1] == msgPunchPkt })
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("slow discovery: %v", elapsed)
	}
	before := received.Load()
	time.Sleep(250 * time.Millisecond)
	if received.Load() != before {
		t.Fatal("discovery kept sending after completion")
	}
}
