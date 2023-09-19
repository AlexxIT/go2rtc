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

	delete(s.sessions, session.Remote.SSRC)

	if len(s.sessions) == 0 {
		_ = s.conn.Close()
	}

	s.mu.Unlock()
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

		switch packetType := b[1]; packetType {
		case 99, 110, 0x80 | 99, 0x80 | 110:
			// this is default position for SSRC in RTP packet
			ssrc := binary.BigEndian.Uint32(b[8:])
			if session, ok := s.sessions[ssrc]; ok {
				session.ReadRTP(b[:n])
			}

		case 200, 201, 202, 203, 204, 205, 206, 207:
			// this is default position for SSRC in RTCP packet
			ssrc := binary.BigEndian.Uint32(b[4:])
			if session, ok := s.sessions[ssrc]; ok {
				session.ReadRTCP(b[:n])
			}
		}
	}
}
