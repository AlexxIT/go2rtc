package cs2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func Dial(host, transport string) (*Conn, error) {
	conn, err := handshake(host, transport)
	if err != nil {
		return nil, err
	}

	_, isTCP := conn.(*tcpConn)

	c := &Conn{
		Conn:  conn,
		isTCP: isTCP,
		channels: [4]*dataChannel{
			newDataChannel(0, 10), nil, newDataChannel(250, 100), nil,
		},
	}
	go c.worker()
	return c, nil
}

// DialCloud establishes a cs2 connection via Mi-Cloud relay (UDP/32100)
// using NAT hole-punching. Required for cameras that do not expose the
// LAN port 32108 directly — typically the MJA1-secure-element generation
// of Xiaomi cameras (e.g. xiaomi.camera.c302n, xiaomi.camera.cw701,
// isa.camera.hlc7), which only stream via Mi-Cloud.
//
// p2pID is the device identifier from the Mi-Cloud vendor_params,
// of the form "PREFIX-NUM-SUFFIX" (e.g. "ABCDEF-123456-GHIJK").
// relayHost is the Mi-Cloud relay IP (one of the published Alibaba Cloud
// pool addresses, e.g. 47.236.156.107, 47.88.30.183, 47.88.30.225).
func DialCloud(p2pID, relayHost string) (*Conn, error) {
	conn, err := cloudHandshake(p2pID, relayHost)
	if err != nil {
		return nil, err
	}

	_, isTCP := conn.(*tcpConn)

	c := &Conn{
		Conn:  conn,
		isTCP: isTCP,
		channels: [4]*dataChannel{
			newDataChannel(0, 10), nil, newDataChannel(250, 100), nil,
		},
	}
	go c.worker()
	return c, nil
}

// cs2CryptoKey is the universal key string used by the CS2-Network
// P2P_Proprietary_Encrypt cipher, extracted from camera firmware
// disassembly. The same constant is found in cameras of multiple
// vendors that license the CS2-Network SDK (see e.g. the VStarcam
// reverse engineering by BrownFineSecurity).
const cs2CryptoKey = "SSD@cs2-network."

// cs2LookupTable is the 256-byte permutation used by the CS2-Network
// cipher. Identical to the table observed in VStarcam firmware.
var cs2LookupTable = [256]byte{
	0x7c, 0x9c, 0xe8, 0x4a, 0x13, 0xde, 0xdc, 0xb2, 0x2f, 0x21, 0x23, 0xe4, 0x30, 0x7b, 0x3d, 0x8c,
	0xbc, 0x0b, 0x27, 0x0c, 0x3c, 0xf7, 0x9a, 0xe7, 0x08, 0x71, 0x96, 0x00, 0x97, 0x85, 0xef, 0xc1,
	0x1f, 0xc4, 0xdb, 0xa1, 0xc2, 0xeb, 0xd9, 0x01, 0xfa, 0xba, 0x3b, 0x05, 0xb8, 0x15, 0x87, 0x83,
	0x28, 0x72, 0xd1, 0x8b, 0x5a, 0xd6, 0xda, 0x93, 0x58, 0xfe, 0xaa, 0xcc, 0x6e, 0x1b, 0xf0, 0xa3,
	0x88, 0xab, 0x43, 0xc0, 0x0d, 0xb5, 0x45, 0x38, 0x4f, 0x50, 0x22, 0x66, 0x20, 0x7f, 0x07, 0x5b,
	0x14, 0x98, 0x1d, 0x9b, 0xa7, 0x2a, 0xb9, 0xa8, 0xcb, 0xf1, 0xfc, 0x49, 0x47, 0x06, 0x3e, 0xb1,
	0x0e, 0x04, 0x3a, 0x94, 0x5e, 0xee, 0x54, 0x11, 0x34, 0xdd, 0x4d, 0xf9, 0xec, 0xc7, 0xc9, 0xe3,
	0x78, 0x1a, 0x6f, 0x70, 0x6b, 0xa4, 0xbd, 0xa9, 0x5d, 0xd5, 0xf8, 0xe5, 0xbb, 0x26, 0xaf, 0x42,
	0x37, 0xd8, 0xe1, 0x02, 0x0a, 0xae, 0x5f, 0x1c, 0xc5, 0x73, 0x09, 0x4e, 0x69, 0x24, 0x90, 0x6d,
	0x12, 0xb3, 0x19, 0xad, 0x74, 0x8a, 0x29, 0x40, 0xf5, 0x2d, 0xbe, 0xa5, 0x59, 0xe0, 0xf4, 0x79,
	0xd2, 0x4b, 0xce, 0x89, 0x82, 0x48, 0x84, 0x25, 0xc6, 0x91, 0x2b, 0xa2, 0xfb, 0x8f, 0xe9, 0xa6,
	0xb0, 0x9e, 0x3f, 0x65, 0xf6, 0x03, 0x31, 0x2e, 0xac, 0x0f, 0x95, 0x2c, 0x5c, 0xed, 0x39, 0xb7,
	0x33, 0x6c, 0x56, 0x7e, 0xb4, 0xa0, 0xfd, 0x7a, 0x81, 0x53, 0x51, 0x86, 0x8d, 0x9f, 0x77, 0xff,
	0x6a, 0x80, 0xdf, 0xe2, 0xbf, 0x10, 0xd7, 0x75, 0x64, 0x57, 0x76, 0xf3, 0x55, 0xcd, 0xd0, 0xc8,
	0x18, 0xe6, 0x36, 0x41, 0x62, 0xcf, 0x99, 0xf2, 0x32, 0x4c, 0x67, 0x60, 0x61, 0x92, 0xca, 0xd3,
	0xea, 0x63, 0x7d, 0x16, 0xb6, 0x8e, 0xd4, 0x68, 0x35, 0xc3, 0x52, 0x9d, 0x46, 0x44, 0x1e, 0x17,
}

// deriveCryptoKey returns the 4-byte stream-cipher state derived from a
// key string. Mirrors the SDK's key-schedule (truncated at 20 bytes).
func deriveCryptoKey(key []byte) [4]byte {
	var c [4]byte
	n := len(key)
	if n > 20 {
		n = 20
	}
	for i := 0; i < n; i++ {
		b := key[i]
		c[0] += b
		c[1] -= b
		c[2] += b / 3
		c[3] ^= b
	}
	return c
}

// selectTableElement returns the keystream byte for the given previous
// ciphertext byte: table[(prev + c_key[prev & 3]) & 0xff].
func selectTableElement(c [4]byte, prev byte) byte {
	return cs2LookupTable[byte(prev+c[prev&3])]
}

// encryptP2P applies the CS2-Network P2P_Proprietary_Encrypt cipher
// to plaintext. The cipher is a stream cipher with previous-ciphertext-byte
// feedback; the same routine decrypts when fed ciphertext, because the
// feedback variable always reads from the cipher side.
func encryptP2P(plain []byte) []byte {
	c := deriveCryptoKey([]byte(cs2CryptoKey))
	out := make([]byte, len(plain))
	var prev byte = 0
	for i, p := range plain {
		out[i] = p ^ selectTableElement(c, prev)
		prev = out[i]
	}
	return out
}

// buildP2PIDBlock encodes a p2p_id string of the form "PREFIX-NUM-SUFFIX"
// into the 20-byte canonical block used by 0x20 (connect) and 0x41 (punch):
//
//	[ 0.. 5] prefix (6 bytes, null-padded)  e.g. "ABCDEF"
//	[ 6.. 7] padding 0x00 0x00
//	[ 8..11] num (uint32 big-endian)        e.g. 123456 -> 0x0001E240
//	[12..16] suffix (5 bytes, null-padded)  e.g. "GHIJK"
//	[17..19] padding 0x00 0x00 0x00
func buildP2PIDBlock(p2pID string) ([]byte, error) {
	parts := strings.Split(p2pID, "-")
	if len(parts) != 3 {
		return nil, fmt.Errorf("cs2 cloud: invalid p2p_id format: %s", p2pID)
	}
	prefix := parts[0]
	numStr := parts[1]
	suffix := parts[2]

	if len(prefix) > 6 {
		return nil, fmt.Errorf("cs2 cloud: p2p_id prefix too long: %s", prefix)
	}
	if len(suffix) > 5 {
		return nil, fmt.Errorf("cs2 cloud: p2p_id suffix too long: %s", suffix)
	}

	num, err := strconv.ParseUint(numStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("cs2 cloud: invalid p2p_id num %q: %w", numStr, err)
	}

	block := make([]byte, 20)
	copy(block[0:6], []byte(prefix))
	binary.BigEndian.PutUint32(block[8:12], uint32(num))
	copy(block[12:17], []byte(suffix))
	return block, nil
}

// buildCloudConnect builds the 0x20 connect-request message (40 bytes total:
// 4-byte header + 36-byte payload, made of the 20-byte p2p_id block and a
// 16-byte nonce/timestamp tail).
func buildCloudConnect(p2pID string) ([]byte, error) {
	block, err := buildP2PIDBlock(p2pID)
	if err != nil {
		return nil, err
	}

	payload := make([]byte, 36)
	copy(payload[0:20], block)
	binary.BigEndian.PutUint32(payload[22:26], uint32(time.Now().Unix()))

	msg := make([]byte, 4+36)
	msg[0] = magic
	msg[1] = msgCloudConnect
	binary.BigEndian.PutUint16(msg[2:4], 36)
	copy(msg[4:], payload)
	return msg, nil
}

// buildCloudPunch builds the 0x41 (msgPunchPkt) frame with the 20-byte
// p2p_id block as payload. In cloud-relay mode the camera silently drops
// the empty 4-byte punch used by the LAN handshake and only responds when
// the punch frame carries the p2p_id block (so the cam can identify which
// session the punch belongs to).
func buildCloudPunch(p2pID string) ([]byte, error) {
	block, err := buildP2PIDBlock(p2pID)
	if err != nil {
		return nil, err
	}

	msg := make([]byte, 4+20)
	msg[0] = magic
	msg[1] = msgPunchPkt
	binary.BigEndian.PutUint16(msg[2:4], 20)
	copy(msg[4:], block)
	return msg, nil
}

// cloudAuthUnknownBlock is the 16-byte payload at plaintext offset 20-35
// of the 0xF9 auth message. Captured verbatim from a Mi-Home iOS
// handshake; the byte pattern (sparse, small values) looks like protocol
// flags or channel constants rather than client-specific data, and
// transmitting it verbatim works against the relay and camera. Zeroing
// this block caused the camera to silently drop subsequent punch packets.
// Values not verified against a second account; treat as opaque.
var cloudAuthUnknownBlock = [16]byte{
	0x00, 0x00, 0x00, 0x06, 0x00, 0x14, 0x00, 0x01,
	0x00, 0x88, 0x00, 0x00, 0x7a, 0x00, 0x00, 0x00,
}

// buildCloudAuth builds the 0xF9 session-auth message (88 bytes total)
// by encrypting an 84-byte plaintext that carries the device_uid block,
// an opaque flags block and the client's own UDP address.
//
// Plaintext layout (84 bytes, verified by decrypting a captured 0xF9):
//
//	[ 0..19] device_uid     = encoded p2p_id (prefix + num_be + suffix)
//	[20..35] unknown        = 16-byte opaque flags/counter block
//	[36..51] local_address  = Xiaomi sockaddr-style: 2B family (00 00) +
//	                          2B port LE + 4B ipv4 reverse-LE + 8B pad
//	[52..67] wan_address    = same format; equal to local when the client
//	                          sits on a public IP without NAT
//	[68..83] device_address = zeros (the camera-side LAN address is
//	                          unknown to us; the relay reports the cam's
//	                          WAN address back in its 0x40 reply anyway)
//
// Zero-filling everything past device_uid produces a working relay
// response (0x21 ACK, 0x40 punch info) but the camera then ignores
// punches — likely because the cam validates the addresses against
// the source it sees on subsequent packets.
func buildCloudAuth(p2pID string, localIP net.IP, localPort uint16) ([]byte, error) {
	uidBlock, err := buildP2PIDBlock(p2pID)
	if err != nil {
		return nil, err
	}

	plain := make([]byte, 84)
	copy(plain[0:20], uidBlock)
	copy(plain[20:36], cloudAuthUnknownBlock[:])

	if v4 := localIP.To4(); v4 != nil && localPort != 0 {
		// local_addr [36..51]
		binary.LittleEndian.PutUint16(plain[38:40], localPort)
		plain[40] = v4[3]
		plain[41] = v4[2]
		plain[42] = v4[1]
		plain[43] = v4[0]
		// wan_addr [52..67] — identical to local for clients without
		// a NAT layer in front (e.g. a server with a public IP).
		binary.LittleEndian.PutUint16(plain[54:56], localPort)
		plain[56] = v4[3]
		plain[57] = v4[2]
		plain[58] = v4[1]
		plain[59] = v4[0]
	}

	cipher := encryptP2P(plain)

	msg := make([]byte, 4+len(cipher))
	msg[0] = magic
	msg[1] = msgCloudAuth
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(cipher)))
	copy(msg[4:], cipher)
	return msg, nil
}

// detectOutboundIP returns the local IPv4 address the OS would use to
// reach dest. Uses Go's UDP-dial trick — no packet is sent, only the
// routing table is consulted. Returns nil on error, in which case
// buildCloudAuth leaves the address fields zeroed.
func detectOutboundIP(dest string) net.IP {
	conn, err := net.Dial("udp", net.JoinHostPort(dest, "1"))
	if err != nil {
		return nil
	}
	defer conn.Close()
	if u, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return u.IP
	}
	return nil
}

// cloudHandshake performs the cs2 cloud-relay handshake against the
// Mi-Cloud relay at relayHost:32100. Steps:
//
//  1. Send 0x20 (connect request) and 0xF9 (encrypted session auth) to
//     the relay; the relay replies with 0x21 (ACK) and 0x40 (NAT-punch
//     info containing the camera's external address).
//  2. Switch the UDP socket's remote to the camera's address from 0x40
//     and send 0x41 (msgPunchPkt) carrying the p2p_id block.
//  3. Wait for 0x42 (P2P-ready UDP) from the camera and return.
//
// Cloud mode is UDP-only: TCP would require TCP hole-punching from both
// sides, which is not feasible behind typical NATs.
func cloudHandshake(p2pID, relayHost string) (net.Conn, error) {
	udp, err := newUDPConn(relayHost, 32100)
	if err != nil {
		return nil, err
	}

	uc := udp.(*udpConn)

	// Determine our own outbound IP + the ephemeral UDP port the socket
	// bound to. These end up in the 0xF9 auth payload; the camera
	// validates them against the source address it sees on subsequent
	// packets and silently drops punches if they are zero-filled.
	var localPort uint16
	if la, ok := uc.UDPConn.LocalAddr().(*net.UDPAddr); ok {
		localPort = uint16(la.Port)
	}
	localIP := detectOutboundIP(relayHost)

	req20, err := buildCloudConnect(p2pID)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	reqF9, err := buildCloudAuth(p2pID, localIP, localPort)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}

	// Send 0x20, briefly wait, then 0xF9. This pacing mirrors the
	// official Mi Home app's handshake timing observed in captures.
	if _, err = uc.Write(req20); err != nil {
		_ = udp.Close()
		return nil, fmt.Errorf("cs2 cloud: send 0x20: %w", err)
	}
	time.Sleep(130 * time.Millisecond)
	if _, err = uc.Write(reqF9); err != nil {
		_ = udp.Close()
		return nil, fmt.Errorf("cs2 cloud: send 0xF9: %w", err)
	}

	// Wait for 0x40 (NAT-punch info). Re-send the connect+auth pair
	// every 2 s in case packets were dropped.
	buf := make([]byte, 1500)
	var natInfo []byte
	deadline := time.Now().Add(15 * time.Second)
	lastResend := time.Now()

	for natInfo == nil {
		if time.Now().After(deadline) {
			_ = udp.Close()
			return nil, fmt.Errorf("cs2 cloud: timeout waiting for 0x40 NAT-punch info")
		}
		if time.Since(lastResend) > 2*time.Second {
			_, _ = uc.Write(req20)
			time.Sleep(130 * time.Millisecond)
			_, _ = uc.Write(reqF9)
			lastResend = time.Now()
		}

		_ = uc.UDPConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := uc.UDPConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			_ = udp.Close()
			return nil, fmt.Errorf("cs2 cloud: read relay: %w", err)
		}

		if n < 4 || buf[0] != magic {
			continue
		}

		switch buf[1] {
		case msgCloudAck: // 0x21 — connect was accepted; keep waiting for 0x40
			continue
		case msgNatPunch: // 0x40 — got camera NAT info
			natInfo = make([]byte, n)
			copy(natInfo, buf[:n])
		}
	}

	// 0x40 payload layout: [F1 40 00 10] [flags:2] [port:2 LE] [ip:4 LE] ...
	// IP and port are little-endian; reading them big-endian silently
	// sends the punch to a wrong host.
	if len(natInfo) < 12 {
		_ = udp.Close()
		return nil, fmt.Errorf("cs2 cloud: 0x40 too short (%d bytes)", len(natInfo))
	}
	camPort := binary.LittleEndian.Uint16(natInfo[6:8])
	camIP := net.IPv4(natInfo[11], natInfo[10], natInfo[9], natInfo[8])
	camAddr := &net.UDPAddr{IP: camIP, Port: int(camPort)}

	// Re-target the UDP socket at the camera's NAT-punched address and
	// send the p2p_id-carrying punch frame.
	uc.addr = camAddr
	_ = uc.UDPConn.SetReadDeadline(time.Time{})
	_ = udp.SetDeadline(time.Now().Add(15 * time.Second))

	punch, err := buildCloudPunch(p2pID)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}

	// Accept any of: P2P-ready (good), or punch echoed back (cam
	// acknowledges and keeps punching). msgP2PRdyTCP also unblocks the
	// loop but we stay on UDP regardless (TCP through NAT is not viable).
	res, err := uc.WriteUntil(punch, func(res []byte) bool {
		return res[1] == msgP2PRdyUDP || res[1] == msgP2PRdyTCP || res[1] == msgPunchPkt
	})
	if err != nil {
		_ = udp.Close()
		return nil, fmt.Errorf("cs2 cloud: punch to cam: %w", err)
	}

	// If the camera echoed the punch back, keep punching until we get
	// an actual P2P-ready signal.
	if res[1] == msgPunchPkt {
		_, err = uc.WriteUntil(res, func(res []byte) bool {
			return res[1] == msgP2PRdyUDP || res[1] == msgP2PRdyTCP
		})
		if err != nil {
			_ = udp.Close()
			return nil, fmt.Errorf("cs2 cloud: wait p2p ready: %w", err)
		}
	}

	_ = udp.SetDeadline(time.Time{})
	return udp, nil
}

type Conn struct {
	net.Conn
	isTCP bool

	err    error
	seqCh0 uint16
	seqCh3 uint16

	channels [4]*dataChannel

	cmdMu  sync.Mutex
	cmdAck func()
}

const (
	magic           = 0xF1
	magicDrw        = 0xD1
	magicTCP        = 0x68
	msgCloudConnect = 0x20 // cs2 cloud-relay: client -> relay, session request via p2p_id
	msgCloudAck     = 0x21 // cs2 cloud-relay: relay -> client, ACK for 0x20
	msgLanSearch    = 0x30
	msgNatPunch     = 0x40 // cs2 cloud-relay: relay -> peer, NAT-punch addresses
	msgPunchPkt     = 0x41
	msgP2PRdyUDP    = 0x42
	msgP2PRdyTCP    = 0x43
	msgDrw          = 0xD0
	msgDrwAck       = 0xD1
	msgPing         = 0xE0
	msgPong         = 0xE1
	msgClose        = 0xF0
	msgCloseAck     = 0xF1
	msgCloudAuth    = 0xF9 // cs2 cloud-relay: client -> relay, encrypted session auth
)

func handshake(host, transport string) (net.Conn, error) {
	conn, err := newUDPConn(host, 32108)
	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := []byte{magic, msgLanSearch, 0, 0}
	res, err := conn.(*udpConn).WriteUntil(req, func(res []byte) bool {
		return res[1] == msgPunchPkt
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	var msgUDP, msgTCP byte

	if transport == "" || transport == "udp" {
		msgUDP = msgP2PRdyUDP
	}
	if transport == "" || transport == "tcp" {
		msgTCP = msgP2PRdyTCP
	}

	res, err = conn.(*udpConn).WriteUntil(res, func(res []byte) bool {
		return res[1] == msgUDP || res[1] == msgTCP
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	if res[1] == msgTCP {
		_ = conn.Close()
		//host := fmt.Sprintf("%d.%d.%d.%d:%d", b[31], b[30], b[29], b[28], uint16(b[27])<<8|uint16(b[26]))
		return newTCPConn(conn.RemoteAddr().String())
	}

	return conn, nil
}

func (c *Conn) worker() {
	defer func() {
		c.channels[0].Close()
		c.channels[2].Close()
	}()

	var keepaliveTS time.Time // only for TCP

	buf := make([]byte, 1200)

	for {
		n, err := c.Conn.Read(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "cs2", err)
			return
		}

		// 0  f1d0  magic
		// 2  005d  size = total size + 4
		// 4  d1    magic
		// 5  00    channel
		// 6  0000  seq
		switch buf[1] {
		case msgDrw:
			ch := buf[5]
			channel := c.channels[ch]

			if c.isTCP {
				// For TCP we should send ping every second to keep connection alive.
				// Based on PCAP analysis: official Mi Home app sends PING every ~1s.
				if now := time.Now(); now.After(keepaliveTS) {
					_, _ = c.Conn.Write([]byte{magic, msgPing, 0, 0})
					keepaliveTS = now.Add(time.Second)
				}

				err = channel.Push(buf[8:n])
			} else {
				var pushed int

				seqHI, seqLO := buf[6], buf[7]
				seq := uint16(seqHI)<<8 | uint16(seqLO)
				pushed, err = channel.PushSeq(seq, buf[8:n])

				if pushed >= 0 {
					// For UDP we should send ACK.
					ack := []byte{magic, msgDrwAck, 0, 6, magicDrw, ch, 0, 1, seqHI, seqLO}
					_, _ = c.Conn.Write(ack)
				}
			}

			if err != nil {
				c.err = fmt.Errorf("%s: %w", "cs2", err)
				return
			}

		case msgPing:
			_, _ = c.Conn.Write([]byte{magic, msgPong, 0, 0})
		case msgPong, msgP2PRdyUDP, msgP2PRdyTCP, msgClose, msgCloseAck: // skip it
		case msgDrwAck: // only for UDP
			if c.cmdAck != nil {
				c.cmdAck()
			}
		default:
			fmt.Printf("%s: unknown msg: %x\n", "cs2", buf[:n])
		}
	}
}

func (c *Conn) Protocol() string {
	if c.isTCP {
		return "cs2+tcp"
	}
	return "cs2+udp"
}

func (c *Conn) Version() string {
	return "CS2"
}

func (c *Conn) Error() error {
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *Conn) ReadCommand() (cmd uint32, data []byte, err error) {
	buf, ok := c.channels[0].Pop()
	if !ok {
		return 0, nil, c.Error()
	}
	cmd = binary.LittleEndian.Uint32(buf)
	data = buf[4:]
	return
}

func (c *Conn) WriteCommand(cmd uint32, data []byte) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	req := marshalCmd(0, c.seqCh0, cmd, data)
	c.seqCh0++

	if c.isTCP {
		_, err := c.Conn.Write(req)
		return err
	}

	var repeat atomic.Int32
	repeat.Store(5)

	timeout := time.NewTicker(time.Second)
	defer timeout.Stop()

	c.cmdAck = func() {
		repeat.Store(0)
		timeout.Reset(1)
	}

	for {
		if _, err := c.Conn.Write(req); err != nil {
			return err
		}
		<-timeout.C
		r := repeat.Add(-1)
		if r < 0 {
			return nil
		}
		if r == 0 {
			return fmt.Errorf("%s: can't send command %d", "cs2", cmd)
		}
	}
}

const hdrSize = 32

func (c *Conn) ReadPacket() (hdr, payload []byte, err error) {
	data, ok := c.channels[2].Pop()
	if !ok {
		return nil, nil, c.Error()
	}
	return data[:hdrSize], data[hdrSize:], nil
}

func (c *Conn) WritePacket(hdr, payload []byte) error {
	const offset = 12

	n := hdrSize + uint32(len(payload))
	req := make([]byte, n+offset)
	req[0] = magic
	req[1] = msgDrw
	binary.BigEndian.PutUint16(req[2:], uint16(n+8))

	req[4] = magicDrw
	req[5] = 3 // channel
	binary.BigEndian.PutUint16(req[6:], c.seqCh3)
	c.seqCh3++
	binary.BigEndian.PutUint32(req[8:], n)
	copy(req[offset:], hdr)
	copy(req[offset+hdrSize:], hdr)

	_, err := c.Conn.Write(req)
	return err
}

func marshalCmd(channel byte, seq uint16, cmd uint32, payload []byte) []byte {
	size := len(payload)
	req := make([]byte, 4+4+4+4+size)

	// 1. message header (4 bytes)
	req[0] = magic
	req[1] = msgDrw
	binary.BigEndian.PutUint16(req[2:], uint16(4+4+4+size))

	// 2. drw? header (4 bytes)
	req[4] = magicDrw
	req[5] = channel
	binary.BigEndian.PutUint16(req[6:], seq)

	// 3. payload size (4 bytes)
	binary.BigEndian.PutUint32(req[8:], uint32(4+size))

	// 4. payload command (4 bytes)
	binary.BigEndian.PutUint32(req[12:], cmd)

	// 5. payload
	copy(req[16:], payload)

	return req
}

func newUDPConn(host string, port int) (net.Conn, error) {
	// We using raw net.UDPConn, because RemoteAddr should be changed during handshake.
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		addr = &net.UDPAddr{IP: net.ParseIP(host), Port: port}
	}

	return &udpConn{UDPConn: conn, addr: addr}, nil
}

type udpConn struct {
	*net.UDPConn
	addr *net.UDPAddr
}

func (c *udpConn) Read(b []byte) (n int, err error) {
	var addr *net.UDPAddr
	for {
		n, addr, err = c.UDPConn.ReadFromUDP(b)
		if err != nil {
			return 0, err
		}

		if string(addr.IP) == string(c.addr.IP) || n >= 8 {
			//log.Printf("<- %x", b[:n])
			return
		}
	}
}

func (c *udpConn) Write(b []byte) (n int, err error) {
	//log.Printf("-> %x", b)
	return c.UDPConn.WriteToUDP(b, c.addr)
}

func (c *udpConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *udpConn) WriteUntil(req []byte, ok func(res []byte) bool) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if _, err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	buf := make([]byte, 1200)

	for {
		n, addr, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		if ok(buf[:n]) {
			c.addr.Port = addr.Port
			return buf[:n], nil
		}
	}
}

func newTCPConn(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	return &tcpConn{conn.(*net.TCPConn), bufio.NewReader(conn)}, nil
}

type tcpConn struct {
	*net.TCPConn
	rd *bufio.Reader
}

func (c *tcpConn) Read(p []byte) (n int, err error) {
	tmp := make([]byte, 8)
	if _, err = io.ReadFull(c.rd, tmp); err != nil {
		return
	}
	n = int(binary.BigEndian.Uint16(tmp))
	if len(p) < n {
		return 0, fmt.Errorf("tcp: buffer too small")
	}
	_, err = io.ReadFull(c.rd, p[:n])
	//log.Printf("<- %x%x", tmp, p[:n])
	return
}

func (c *tcpConn) Write(req []byte) (n int, err error) {
	n = len(req)
	buf := make([]byte, 8+n)
	binary.BigEndian.PutUint16(buf, uint16(n))
	buf[2] = magicTCP
	copy(buf[8:], req)
	//log.Printf("-> %x", buf)
	_, err = c.TCPConn.Write(buf)
	return
}

func newDataChannel(pushSize, popSize int) *dataChannel {
	c := &dataChannel{}
	if pushSize > 0 {
		c.pushBuf = make(map[uint16][]byte, pushSize)
		c.pushSize = pushSize
	}
	if popSize >= 0 {
		c.popBuf = make(chan []byte, popSize)
	}
	return c
}

type dataChannel struct {
	waitSeq  uint16
	pushBuf  map[uint16][]byte
	pushSize int

	waitData []byte
	waitSize int
	popBuf   chan []byte
}

func (c *dataChannel) Push(b []byte) error {
	c.waitData = append(c.waitData, b...)

	for len(c.waitData) > 4 {
		// Every new data starts with size. There can be several data inside one packet.
		if c.waitSize == 0 {
			c.waitSize = int(binary.BigEndian.Uint32(c.waitData))
			c.waitData = c.waitData[4:]
		}
		if c.waitSize > len(c.waitData) {
			break
		}

		select {
		case c.popBuf <- c.waitData[:c.waitSize]:
		default:
			return fmt.Errorf("pop buffer is full")
		}

		c.waitData = c.waitData[c.waitSize:]
		c.waitSize = 0
	}
	return nil
}

func (c *dataChannel) Pop() ([]byte, bool) {
	data, ok := <-c.popBuf
	return data, ok
}

func (c *dataChannel) Close() {
	close(c.popBuf)
}

// PushSeq returns how many seq were processed.
// Returns 0 if seq was saved or processed earlier.
// Returns -1 if seq could not be saved (buffer full or disabled).
func (c *dataChannel) PushSeq(seq uint16, data []byte) (int, error) {
	diff := int16(seq - c.waitSeq)
	// Check if this is seq from the future.
	if diff > 0 {
		// Support disabled buffer.
		if c.pushSize == 0 {
			return -1, nil // couldn't save seq
		}
		// Check if we don't have this seq in the buffer.
		if c.pushBuf[seq] == nil {
			// Check if there is enough space in the buffer.
			if len(c.pushBuf) == c.pushSize {
				return -1, nil // couldn't save seq
			}
			c.pushBuf[seq] = bytes.Clone(data)
			//log.Printf("push buf wait=%d seq=%d len=%d", c.waitSeq, seq, len(c.pushBuf))
		}
		return 0, nil
	}

	// Check if this is seq from the past.
	if diff < 0 {
		return 0, nil
	}

	for i := 1; ; i++ {
		if err := c.Push(data); err != nil {
			return i, err
		}
		c.waitSeq++
		// Check if we have next seq in the buffer.
		if data = c.pushBuf[c.waitSeq]; data != nil {
			delete(c.pushBuf, c.waitSeq)
		} else {
			return i, nil
		}
	}
}
