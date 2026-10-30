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

// initKey is the fixed CS2 table for decoding the P2P init string
var initKey = []byte{
	0x49, 0x59, 0x43, 0x3d, 0xb5, 0xbf, 0x6d, 0xa3, 0x47, 0x53, 0x4f, 0x61, 0x65, 0xe3,
	0x71, 0xe9, 0x67, 0x7f, 0x02, 0x03, 0x0b, 0xad, 0xb3, 0x89, 0x2b, 0x2f, 0x35, 0xc1,
	0x6b, 0x8b, 0x95, 0x97, 0x11, 0xe5, 0xa7, 0x0d, 0xef, 0xf1, 0x05, 0x07, 0x83, 0xfb,
	0x9d, 0x3b, 0xc5, 0xc7, 0x13, 0x17, 0x1d, 0x1f, 0x25, 0x29, 0xd3, 0xdf,
}

// DecodeInitString returns P2P server addresses from the init_string
// of miss_get_vendor response, e.g. "EEGGFCBN..." => "42.157.163.61,..."
func DecodeInitString(s string) ([]string, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("cs2: wrong init string")
	}

	b := make([]byte, 0, len(s)/2)
	z := byte(0x39)
	for i := 0; i < len(s); i += 2 {
		hi, lo := s[i]-'A', s[i+1]-'A'
		if hi > 0xF || lo > 0xF {
			return nil, fmt.Errorf("cs2: wrong init string")
		}
		c := (hi<<4 | lo) ^ initKey[len(b)%len(initKey)] ^ z
		b = append(b, c)
		z ^= c
	}

	var servers []string
	for _, host := range strings.Split(string(b), ",") {
		if host != "" {
			servers = append(servers, host)
		}
	}
	return servers, nil
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
