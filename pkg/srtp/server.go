package srtp

import (
	"encoding/binary"
	"net"
	"strconv"
	"sync"
)

type Server struct {
	address  string
	conn     net.PacketConn
	sessions map[uint32]*Session
	mu       sync.Mutex
}

func NewServer(address string) *Server {
	return &Server{
		address:  address,
		sessions: map[uint32]*Session{},
	}
}

func (s *Server) Port() int {
	if s.conn != nil {
		return s.conn.LocalAddr().(*net.UDPAddr).Port
	}

	_, a, _ := net.SplitHostPort(s.address)
	i, _ := strconv.Atoi(a)
	return i
}

func (s *Server) AddSession(session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := session.init(); err != nil {
		return
	}

	if len(s.sessions) == 0 {
		var err error
		if s.conn, err = net.ListenPacket("udp", s.address); err != nil {
			return
		}
		go s.handle()
	}

	session.conn = s.conn

	s.sessions[session.Remote.SSRC] = session
}

func (s *Server) DelSession(session *Session) {
	s.mu.Lock()

	for ssrc, current := range s.sessions {
		if current == session {
			delete(s.sessions, ssrc)
		}
	}

	// check s.conn for https://github.com/AlexxIT/go2rtc/issues/734
	if len(s.sessions) == 0 && s.conn != nil {
		_ = s.conn.Close()
	}

	s.mu.Unlock()
}

func (s *Server) GetSession(ssrc uint32) (session *Session) {
	s.mu.Lock()
	session = s.sessions[ssrc]
	s.mu.Unlock()
	return
}

func (s *Server) handle() error {
	b := make([]byte, 2048)
	for {
		n, _, err := s.conn.ReadFrom(b)
		if err != nil {
			return err
		}

		// Multiplexing RTP Data and Control Packets on a Single Port
		// https://datatracker.ietf.org/doc/html/rfc5761

		switch kind, ssrc := packetKindAndSSRC(b[:n]); kind {
		case packetKindRTP:
			if session := s.GetSession(ssrc); session != nil {
				session.ReadRTP(b[:n])
			} else {
				s.ReadRTP(ssrc, b[:n])
			}

		case packetKindRTCP:
			if session := s.GetSession(ssrc); session != nil {
				session.ReadRTCP(b[:n])
			}
		}
	}
}

func (s *Server) ReadRTP(ssrc uint32, b []byte) {
	s.mu.Lock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()

	for _, session := range sessions {
		if session.OnReadRTP == nil {
			continue
		}
		if session.ReadRTP(b) {
			s.mu.Lock()
			s.sessions[ssrc] = session
			s.mu.Unlock()
			return
		}
	}
}

const (
	packetKindUnknown = iota
	packetKindRTP
	packetKindRTCP
)

func packetKindAndSSRC(b []byte) (int, uint32) {
	if len(b) < 2 {
		return packetKindUnknown, 0
	}

	// Multiplexing RTP Data and Control Packets on a Single Port
	// https://datatracker.ietf.org/doc/html/rfc5761
	switch packetType := b[1]; packetType {
	case 200, 201, 202, 203, 204, 205, 206, 207:
		if len(b) < 8 {
			return packetKindUnknown, 0
		}
		return packetKindRTCP, binary.BigEndian.Uint32(b[4:])
	}

	payloadType := b[1] & 0x7F
	switch {
	case payloadType == 13:
		return packetKindUnknown, 0 // comfort noise
	case payloadType >= 64 && payloadType <= 95:
		return packetKindUnknown, 0 // RTCP conflict range
	case len(b) < 12:
		return packetKindUnknown, 0
	}

	return packetKindRTP, binary.BigEndian.Uint32(b[8:])
}
