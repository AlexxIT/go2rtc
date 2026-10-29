package cs2

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Some cameras (e.g. midr.cateye.ph300) don't answer LAN search on port 32108.
// They are only registered on CS2 P2P servers (UDP 32100), and the client must
// ask these servers to introduce it to the camera. The servers reply with the
// camera addresses (0x40), then both sides punch each other. On the same LAN
// the stream still goes directly between client and camera.

const (
	msgP2PReq    = 0x20
	msgP2PReqAck = 0x21
	msgPunchTo   = 0x40
)

// DefaultServers returns known P2P servers for the p2p_id prefix.
func DefaultServers(p2pID string) []string {
	if strings.HasPrefix(p2pID, "XMCN") {
		// observed from midr.cateye.ph300 registrations
		return []string{"110.43.39.45", "110.43.68.105", "42.157.163.61"}
	}
	return nil
}

func DialServers(p2pID string, servers []string, transport string) (*Conn, error) {
	conn, err := serverHandshake(p2pID, servers, transport)
	if err != nil {
		return nil, err
	}
	return newConn(conn), nil
}

func serverHandshake(p2pID string, servers []string, transport string) (net.Conn, error) {
	did, err := marshalDID(p2pID)
	if err != nil {
		return nil, err
	}

	udp, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	var srvAddrs []*net.UDPAddr
	for _, s := range servers {
		if addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(s, "32100")); err == nil {
			srvAddrs = append(srvAddrs, addr)
		}
	}
	if len(srvAddrs) == 0 {
		_ = udp.Close()
		return nil, fmt.Errorf("cs2: no p2p servers")
	}

	localIP := outboundIP(srvAddrs[0].IP.String())
	localPort := uint16(udp.LocalAddr().(*net.UDPAddr).Port)

	reqP2P := marshalP2PReq(did, localIP, localPort)
	punch := marshalMsg(msgPunchPkt, did)

	isServer := func(addr *net.UDPAddr) bool {
		for _, srv := range srvAddrs {
			if srv.IP.Equal(addr.IP) {
				return true
			}
		}
		return false
	}

	var peers []*net.UDPAddr
	addPeer := func(addr *net.UDPAddr) {
		for _, p := range peers {
			if p.IP.Equal(addr.IP) && p.Port == addr.Port {
				return
			}
		}
		peers = append(peers, addr)
	}

	sendAll := func() {
		for _, srv := range srvAddrs {
			_, _ = udp.WriteToUDP(reqP2P, srv)
		}
	}

	acks := map[string]string{}
	deadline := time.Now().Add(15 * time.Second)
	nextSend, nextAsk := time.Now(), time.Now()
	buf := make([]byte, 1500)

	for {
		now := time.Now()
		if now.After(deadline) {
			_ = udp.Close()
			return nil, fmt.Errorf("cs2: no answer from camera, servers acks=%v peers=%v", acks, peers)
		}
		// camera may change its port after wakeup, so keep asking servers
		if now.After(nextAsk) {
			sendAll()
			nextAsk = now.Add(2 * time.Second)
		}
		if now.After(nextSend) {
			for _, p := range peers {
				_, _ = udp.WriteToUDP(punch, p)
			}
			nextSend = now.Add(500 * time.Millisecond)
		}

		_ = udp.SetReadDeadline(now.Add(200 * time.Millisecond))
		n, addr, err := udp.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			_ = udp.Close()
			return nil, err
		}
		if n < 4 || buf[0] != magic {
			continue
		}

		if isServer(addr) {
			switch buf[1] {
			case msgP2PReqAck:
				acks[addr.IP.String()] = hex.EncodeToString(buf[4:n])
			case msgPunchTo:
				if n >= 4+16 {
					if peer := unmarshalAddr(buf[4:20]); peer != nil {
						addPeer(peer)
					}
				}
				nextSend = time.Time{}
			}
			continue
		}

		switch buf[1] {
		case msgPunchPkt:
			addPeer(addr)
			_, _ = udp.WriteToUDP(punch, addr)
			_, _ = udp.WriteToUDP(marshalMsg(msgP2PRdyUDP, did), addr)
		case msgP2PRdyUDP:
			if transport == "tcp" {
				continue
			}
			_ = udp.SetReadDeadline(time.Time{})
			return &udpConn{UDPConn: udp, addr: addr}, nil
		case msgP2PRdyTCP:
			if transport == "udp" {
				continue
			}
			_ = udp.Close()
			return newTCPConn(addr.String())
		}
	}
}

// marshalDID converts "XMCN-123456-ABCDE" to the 20 byte binary form:
// prefix (8 bytes), serial (uint32 BE), check code (8 bytes).
func marshalDID(p2pID string) ([]byte, error) {
	parts := strings.Split(p2pID, "-")
	if len(parts) != 3 || len(parts[0]) > 8 || len(parts[2]) > 8 {
		return nil, fmt.Errorf("cs2: wrong p2p_id: %s", p2pID)
	}
	serial, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("cs2: wrong p2p_id: %s", p2pID)
	}
	b := make([]byte, 20)
	copy(b, parts[0])
	binary.BigEndian.PutUint32(b[8:], uint32(serial))
	copy(b[12:], parts[2])
	return b, nil
}

// marshalAddr - CS2 sockaddr: family (BE), port (LE), IPv4 (reversed), 8 zero bytes.
func marshalAddr(b []byte, ip net.IP, port uint16) {
	ip = ip.To4()
	if ip == nil {
		return
	}
	b[1] = 2 // AF_INET
	binary.LittleEndian.PutUint16(b[2:], port)
	b[4], b[5], b[6], b[7] = ip[3], ip[2], ip[1], ip[0]
}

func unmarshalAddr(b []byte) *net.UDPAddr {
	ip := net.IPv4(b[7], b[6], b[5], b[4])
	if ip.IsUnspecified() {
		return nil
	}
	return &net.UDPAddr{IP: ip, Port: int(binary.LittleEndian.Uint16(b[2:]))}
}

func marshalMsg(msgType byte, payload []byte) []byte {
	b := make([]byte, 4+len(payload))
	b[0] = magic
	b[1] = msgType
	binary.BigEndian.PutUint16(b[2:], uint16(len(payload)))
	copy(b[4:], payload)
	return b
}

// marshalP2PReq - DID (20 bytes) + client local sockaddr (16 bytes).
func marshalP2PReq(did []byte, ip net.IP, port uint16) []byte {
	payload := make([]byte, 36)
	copy(payload, did)
	marshalAddr(payload[20:], ip, port)
	return marshalMsg(msgP2PReq, payload)
}

func outboundIP(dest string) net.IP {
	conn, err := net.Dial("udp4", net.JoinHostPort(dest, "1"))
	if err != nil {
		return nil
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}
