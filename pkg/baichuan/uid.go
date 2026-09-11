package baichuan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"net"
	"sync"
	"time"
)

type uidSession struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	mtu        int
	clientID   int32
	cameraID   int32
	readQueue  chan []byte
	writeQueue chan []byte
	closeCh    chan struct{}
	closeOnce  sync.Once
	wg         sync.WaitGroup
	closeState closeState

	readMu  sync.Mutex
	readBuf bytes.Buffer

	sentMu       sync.Mutex
	sentPackets  map[uint32][]byte
	nextPacketID uint32

	recvMu      sync.Mutex
	recvPackets map[uint32][]byte
	consumedID  uint32
	hasConsumed bool
}

func dialUIDLocal(ctx context.Context, uid string, timeout time.Duration) (*uidSession, error) {
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}

	if err := enableBroadcast(udpConn); err != nil {
		udpConn.Close()
		return nil, err
	}

	localPort := udpConn.LocalAddr().(*net.UDPAddr).Port

	tid := uint32(randomUint8())
	clientID := randomInt32()

	xmlPayload, err := marshalUDPXML(udpP2PEnvelope{
		C2DC: &udpC2DC{
			UID: uid,
			Client: udpPortList{
				Port: localPort,
			},
			CID:   clientID,
			MTU:   defaultUIDMTU,
			Debug: 0,
			OS:    "MAC",
		},
	})
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	discoveryPacket, err := marshalUDPPacket(udpDiscoveryPacket{
		TID:     tid,
		Payload: UDPXOR(tid, xmlPayload),
	})
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	broadcasts := ipv4Broadcasts()
	deadline := time.Now().Add(timeout)
	nextSend := time.Time{}

	for {
		if err := ctx.Err(); err != nil {
			udpConn.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			udpConn.Close()
			return nil, fmt.Errorf("uid discovery timed out")
		}

		if nextSend.IsZero() || time.Now().After(nextSend) {
			for _, ip := range broadcasts {
				for _, port := range []int{2015, 2018} {
					_, _ = udpConn.WriteToUDP(discoveryPacket, &net.UDPAddr{IP: ip, Port: port})
				}
			}
			nextSend = time.Now().Add(500 * time.Millisecond)
		}

		readDeadline := nextSend
		if deadline.Before(readDeadline) {
			readDeadline = deadline
		}
		if err := udpConn.SetReadDeadline(readDeadline); err != nil {
			udpConn.Close()
			return nil, err
		}

		buf := make([]byte, defaultUIDMTU)
		n, addr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			udpConn.Close()
			return nil, err
		}

		packet, err := parseUDPPacket(buf[:n])
		if err != nil {
			continue
		}

		discovery, ok := packet.(udpDiscoveryPacket)
		if !ok {
			continue
		}

		decrypted := UDPXOR(discovery.TID, discovery.Payload)
		var envelope udpP2PEnvelope
		if err := xml.Unmarshal(decrypted, &envelope); err != nil {
			continue
		}
		if envelope.D2CCR == nil || envelope.D2CCR.CID != clientID {
			continue
		}

		session := &uidSession{
			conn:        udpConn,
			remoteAddr:  addr,
			mtu:         int(defaultUIDMTU),
			clientID:    clientID,
			cameraID:    envelope.D2CCR.DID,
			readQueue:   make(chan []byte, 128),
			writeQueue:  make(chan []byte, 128),
			closeCh:     make(chan struct{}),
			sentPackets: make(map[uint32][]byte),
			recvPackets: make(map[uint32][]byte),
		}
		session.wg.Add(2)
		go session.readLoop()
		go session.writeLoop()
		return session, nil
	}
}

func (s *uidSession) Read(p []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	for s.readBuf.Len() == 0 {
		select {
		case chunk, ok := <-s.readQueue:
			if ok {
				s.readBuf.Write(chunk)
				continue
			}
			if err := s.closeState.get(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		case <-s.closeCh:
			if err := s.closeState.get(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
	}

	return s.readBuf.Read(p)
}

func (s *uidSession) Write(p []byte) (int, error) {
	maxPayload := s.mtu - 20
	if maxPayload <= 0 {
		return 0, errors.New("invalid uid mtu")
	}

	written := 0
	for len(p) > 0 {
		chunkLen := len(p)
		if chunkLen > maxPayload {
			chunkLen = maxPayload
		}

		chunk := append([]byte(nil), p[:chunkLen]...)
		select {
		case s.writeQueue <- chunk:
		case <-s.closeCh:
			if err := s.closeState.get(); err != nil {
				return written, err
			}
			return written, io.ErrClosedPipe
		}

		p = p[chunkLen:]
		written += chunkLen
	}

	return written, nil
}

func (s *uidSession) Close() error {
	s.shutdown(io.EOF)
	s.wg.Wait()
	return nil
}

func (s *uidSession) shutdown(err error) {
	s.closeOnce.Do(func() {
		s.closeState.set(err)
		close(s.closeCh)
		_ = s.conn.Close()
	})
}

func (s *uidSession) readLoop() {
	defer s.wg.Done()
	defer close(s.readQueue)

	buf := make([]byte, s.mtu)
	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		_ = s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			s.shutdown(err)
			return
		}

		if s.remoteAddr != nil && !addr.IP.Equal(s.remoteAddr.IP) {
			continue
		}
		if s.remoteAddr != nil && addr.Port != s.remoteAddr.Port {
			continue
		}

		packet, err := parseUDPPacket(buf[:n])
		if err != nil {
			continue
		}

		switch pkt := packet.(type) {
		case udpDiscoveryPacket:
			decrypted := UDPXOR(pkt.TID, pkt.Payload)
			var envelope udpP2PEnvelope
			if err := xml.Unmarshal(decrypted, &envelope); err != nil {
				continue
			}
			if envelope.D2CDisc != nil && envelope.D2CDisc.CID == s.clientID && envelope.D2CDisc.DID == s.cameraID {
				s.shutdown(fmt.Errorf("camera requested uid disconnect"))
				return
			}

		case udpAckPacket:
			if pkt.ConnectionID != s.clientID {
				continue
			}
			s.sentMu.Lock()
			for packetID := range s.sentPackets {
				if packetID <= pkt.PacketID {
					delete(s.sentPackets, packetID)
				}
			}
			for idx, value := range pkt.Payload {
				if value == 0 {
					continue
				}
				delete(s.sentPackets, pkt.PacketID+1+uint32(idx))
			}
			s.sentMu.Unlock()

		case udpDataPacket:
			if pkt.ConnectionID != s.clientID {
				continue
			}

			s.recvMu.Lock()
			if _, ok := s.recvPackets[pkt.PacketID]; !ok {
				s.recvPackets[pkt.PacketID] = append([]byte(nil), pkt.Payload...)
			}
			chunks := contiguousPayloads(s.recvPackets, &s.consumedID, &s.hasConsumed)
			s.recvMu.Unlock()

			for _, chunk := range chunks {
				select {
				case s.readQueue <- chunk:
				case <-s.closeCh:
					return
				}
			}
		}
	}
}

func (s *uidSession) writeLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeCh:
			return

		case chunk := <-s.writeQueue:
			if chunk == nil {
				continue
			}

			s.sentMu.Lock()
			packetID := s.nextPacketID
			s.nextPacketID++
			s.sentMu.Unlock()

			packet, err := marshalUDPPacket(udpDataPacket{
				ConnectionID: s.cameraID,
				PacketID:     packetID,
				Payload:      chunk,
			})
			if err != nil {
				s.shutdown(err)
				return
			}

			if _, err := s.conn.WriteToUDP(packet, s.remoteAddr); err != nil {
				s.shutdown(err)
				return
			}

			s.sentMu.Lock()
			s.sentPackets[packetID] = packet
			s.sentMu.Unlock()

		case <-ticker.C:
			s.sentMu.Lock()
			for _, packet := range s.sentPackets {
				_, _ = s.conn.WriteToUDP(packet, s.remoteAddr)
			}
			s.sentMu.Unlock()

			s.recvMu.Lock()
			start, payload, ok := ackWindow(s.recvPackets, s.consumedID, s.hasConsumed)
			s.recvMu.Unlock()
			if !ok {
				continue
			}

			packet, err := marshalUDPPacket(udpAckPacket{
				ConnectionID: s.cameraID,
				PacketID:     start,
				Payload:      payload,
			})
			if err != nil {
				s.shutdown(err)
				return
			}
			if _, err := s.conn.WriteToUDP(packet, s.remoteAddr); err != nil {
				s.shutdown(err)
				return
			}
		}
	}
}

func ipv4Broadcasts() []net.IP {
	set := map[string]net.IP{
		net.IPv4bcast.String(): net.IPv4bcast,
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{net.IPv4bcast}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP == nil || ipnet.IP.To4() == nil || ipnet.Mask == nil {
				continue
			}
			ip4 := ipnet.IP.To4()
			mask := ipnet.Mask
			if len(mask) != net.IPv4len {
				continue
			}
			bcast := net.IPv4(
				ip4[0]|^mask[0],
				ip4[1]|^mask[1],
				ip4[2]|^mask[2],
				ip4[3]|^mask[3],
			)
			set[bcast.String()] = bcast
		}
	}

	out := make([]net.IP, 0, len(set))
	for _, ip := range set {
		out = append(out, ip)
	}
	return out
}

func enableBroadcast(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var ctrlErr error
	if err := raw.Control(func(fd uintptr) {
		ctrlErr = setBroadcastOption(fd)
	}); err != nil {
		return err
	}
	return ctrlErr
}

func randomUint8() uint8 {
	var b [1]byte
	if _, err := rand.Read(b[:]); err == nil {
		return b[0]
	}
	return uint8(mathrand.Intn(math.MaxUint8 + 1)) //#nosec G404 G115
}

func randomInt32() int32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil {
		return int32(binary.LittleEndian.Uint32(b[:])) //#nosec G115
	}
	return int32(mathrand.Uint32()) //#nosec G404 G115
}
