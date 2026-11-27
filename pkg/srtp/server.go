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

	if s.sessions[session.Remote.SSRC] == session {
		delete(s.sessions, session.Remote.SSRC)
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

		rtcp, ssrc, ok := parseSSRC(b[:n])
		if !ok {
			continue
		}

		session := s.GetSession(ssrc)
		if session == nil {
			continue
		}

		if rtcp {
			session.ReadRTCP(b[:n])
		} else {
			session.ReadRTP(b[:n])
		}
	}
}

// parseSSRC returns the SSRC of an RTP or RTCP packet on a multiplexed port.
// RTCP packet types 200-207 can't be RTP payload types 72-79 with the marker
// bit (RFC 5761 section 4), so everything else is RTP whatever the payload type.
func parseSSRC(b []byte) (rtcp bool, ssrc uint32, ok bool) {
	if len(b) < 12 {
		return false, 0, false
	}
	if packetType := b[1]; packetType >= 200 && packetType <= 207 {
		// this is default position for SSRC in RTCP packet
		return true, binary.BigEndian.Uint32(b[4:]), true
	}
	// this is default position for SSRC in RTP packet
	return false, binary.BigEndian.Uint32(b[8:]), true
}
