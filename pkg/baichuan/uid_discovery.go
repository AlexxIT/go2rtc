package baichuan

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

const (
	uidMTU               = 1350
	uidDiscoveryInterval = 500 * time.Millisecond
)

var uidDiscoveryPorts = [...]uint16{2015, 2018}

type uidEnvelope struct {
	XMLName  xml.Name     `xml:"P2P"`
	Connect  *uidConnect  `xml:"C2D_C,omitempty"`
	Response *uidResponse `xml:"D2C_C_R,omitempty"`
}

type uidConnect struct {
	UID    string      `xml:"uid"`
	Client uidPortList `xml:"cli"`
	CID    int32       `xml:"cid"`
	MTU    int         `xml:"mtu"`
	Debug  int         `xml:"debug"`
	OS     string      `xml:"p"`
}

type uidPortList struct {
	Port int `xml:"port"`
}

type uidResponse struct {
	CID   int32  `xml:"cid"`
	DID   int32  `xml:"did"`
	Rsp   int    `xml:"rsp"`
	Timer string `xml:"timer"`
}

type uidDiscovery struct {
	conn     *net.UDPConn
	remote   netip.AddrPort
	clientID int32
	cameraID int32
}

func discoverUID(ctx context.Context, uid, local, broadcast string, timeout time.Duration) (*uidDiscovery, error) {
	if !validUID(uid) {
		return nil, fmt.Errorf("baichuan: invalid UID")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("baichuan: UID discovery timeout must be positive")
	}
	var err error
	var localAddr netip.Addr
	if local != "" {
		localAddr, err = netip.ParseAddr(strings.TrimSpace(local))
		if err != nil || !uidUnicastAddr(localAddr) {
			return nil, fmt.Errorf("baichuan: invalid UID local address")
		}
	}
	var broadcastAddr netip.Addr
	if broadcast != "" {
		broadcastAddr, err = netip.ParseAddr(strings.TrimSpace(broadcast))
		if err != nil || !uidDiscoveryAddr(broadcastAddr) {
			return nil, fmt.Errorf("baichuan: invalid UID broadcast address")
		}
	}
	broadcasts, err := uidBroadcasts(localAddr, broadcastAddr)
	if err != nil {
		return nil, err
	}

	listenIP := net.IPv4zero
	if localAddr.IsValid() {
		listenIP = net.IP(localAddr.AsSlice())
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: listenIP})
	if err != nil {
		return nil, fmt.Errorf("baichuan: listen for UID discovery: %w", err)
	}
	defer interruptDeadline(ctx, conn.SetDeadline)()
	if err = enableUIDBroadcast(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("baichuan: enable UID broadcast: %w", err)
	}

	clientID, transaction, err := uidRandomIDs()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	payload, err := xml.Marshal(uidEnvelope{Connect: &uidConnect{
		UID: uid, Client: uidPortList{Port: conn.LocalAddr().(*net.UDPAddr).Port},
		CID: clientID, MTU: uidMTU, OS: "MAC",
	}})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("baichuan: marshal UID discovery: %w", err)
	}
	payload = append([]byte(xml.Header), payload...)
	xorUID(payload, payload, transaction)
	packet, err := marshalUIDDiscovery(transaction, payload)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err = conn.SetWriteDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("baichuan: set UID discovery write deadline: %w", err)
	}
	for {
		if err = ctx.Err(); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if !time.Now().Before(deadline) {
			_ = conn.Close()
			return nil, fmt.Errorf("baichuan: UID discovery timed out")
		}
		var sendErr error
		sent := false
		for _, address := range broadcasts {
			for _, port := range uidDiscoveryPorts {
				if _, writeErr := conn.WriteToUDPAddrPort(packet, netip.AddrPortFrom(address, port)); writeErr != nil {
					sendErr = writeErr
				} else {
					sent = true
				}
			}
		}
		if !sent {
			_ = conn.Close()
			return nil, fmt.Errorf("baichuan: broadcast UID discovery: %w", sendErr)
		}
		if discovery, ok := readUIDDiscovery(conn, deadline, clientID, transaction); ok {
			return discovery, nil
		}
	}
}

func validUID(uid string) bool {
	return len(uid) != 0 && len(uid) <= 64 && strings.IndexFunc(uid, func(r rune) bool {
		return !('0' <= r && r <= '9' || 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z')
	}) < 0
}

func uidUnicastAddr(address netip.Addr) bool {
	return address.Is4() && !address.IsLoopback() &&
		(address.IsGlobalUnicast() || address.IsLinkLocalUnicast())
}

func uidDiscoveryAddr(address netip.Addr) bool {
	return address.Is4() && !address.IsUnspecified() && !address.IsLoopback() && !address.IsMulticast()
}

func readUIDDiscovery(conn *net.UDPConn, deadline time.Time, clientID int32, transaction uint32) (*uidDiscovery, bool) {
	readDeadline := time.Now().Add(uidDiscoveryInterval)
	if deadline.Before(readDeadline) {
		readDeadline = deadline
	}
	_ = conn.SetReadDeadline(readDeadline)
	b := make([]byte, uidMTU)
	for {
		n, remote, err := conn.ReadFromUDPAddrPort(b)
		if err != nil {
			return nil, false
		}
		cameraID, ok := parseUIDResponse(b[:n], remote, clientID, transaction)
		if !ok {
			continue
		}
		return &uidDiscovery{
			conn: conn, remote: remote, clientID: clientID, cameraID: cameraID,
		}, true
	}
}

func parseUIDResponse(b []byte, remote netip.AddrPort, clientID int32, transaction uint32) (int32, bool) {
	packet, err := parseUIDPacket(b, uidMTU-uidDiscoveryHeader)
	if err != nil || packet.magic != uidMagicDiscovery || packet.transaction != transaction ||
		!uidUnicastAddr(remote.Addr()) || remote.Port() == 0 {
		return 0, false
	}
	xorUID(packet.payload, packet.payload, packet.transaction)
	var envelope uidEnvelope
	if xml.Unmarshal(packet.payload, &envelope) != nil || envelope.Response == nil ||
		envelope.Response.CID != clientID || envelope.Response.DID <= 0 || envelope.Response.Rsp != 0 {
		return 0, false
	}
	return envelope.Response.DID, true
}

func uidBroadcasts(local, broadcast netip.Addr) ([]netip.Addr, error) {
	if broadcast.IsValid() {
		return []netip.Addr{broadcast}, nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("baichuan: list UID interfaces: %w", err)
	}
	set := make(map[netip.Addr]struct{})
	for _, iface := range interfaces {
		if iface.Flags&(net.FlagUp|net.FlagBroadcast) != net.FlagUp|net.FlagBroadcast ||
			iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("baichuan: list addresses for UID interface: %w", err)
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil || !prefix.Addr().Is4() || prefix.Bits() >= 31 {
				continue
			}
			if local.IsValid() && prefix.Addr() != local {
				continue
			}
			base := prefix.Masked().Addr().As4()
			bits := prefix.Bits()
			value := binary.BigEndian.Uint32(base[:]) | uint32(1<<(32-bits)-1)
			var broadcast [4]byte
			binary.BigEndian.PutUint32(broadcast[:], value)
			set[netip.AddrFrom4(broadcast)] = struct{}{}
		}
	}
	if len(set) == 0 {
		if local.IsValid() {
			return nil, fmt.Errorf("baichuan: UID local address has no broadcast interface")
		}
		return nil, fmt.Errorf("baichuan: no IPv4 broadcast interface")
	}
	broadcasts := make([]netip.Addr, 0, len(set))
	for address := range set {
		broadcasts = append(broadcasts, address)
	}
	return broadcasts, nil
}

func uidRandomIDs() (int32, uint32, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, 0, fmt.Errorf("baichuan: generate UID identifiers: %w", err)
	}
	clientID := int32(binary.LittleEndian.Uint32(b[:4]) & 0x7fffffff)
	if clientID == 0 {
		clientID = 1
	}
	return clientID, binary.LittleEndian.Uint32(b[4:]), nil
}

func enableUIDBroadcast(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var optionErr error
	if err = raw.Control(func(fd uintptr) {
		optionErr = setUIDBroadcast(fd)
	}); err != nil {
		return err
	}
	return optionErr
}
