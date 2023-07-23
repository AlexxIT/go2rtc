package srtp

import (
	"encoding/binary"
	"net"
	"sync/atomic"
)

// Server using same UDP port for SRTP and for SRTCP as the iPhone does
// this is not really necessary but anyway
type Server struct {
	conn     net.PacketConn
	sessions map[uint32]*Session
}

func (s *Server) Port() uint16 {
	addr := s.conn.LocalAddr().(*net.UDPAddr)
	return uint16(addr.Port)
}

func (s *Server) Close() error {
	return s.conn.Close()
}

func (s *Server) AddSession(session *Session) {
	if s.sessions == nil {
		s.sessions = map[uint32]*Session{}
	}
	s.sessions[session.RemoteSSRC] = session
}

func (s *Server) RemoveSession(session *Session) {
	delete(s.sessions, session.RemoteSSRC)
}

func (s *Server) Serve(conn net.PacketConn) error {
	s.conn = conn

	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return err
		}

		if s.sessions == nil {
			continue
		}

		// Multiplexing RTP Data and Control Packets on a Single Port
		// https://datatracker.ietf.org/doc/html/rfc5761

		var handle func([]byte) error

		// this is default position for SSRC in RTP packet
		ssrc := binary.BigEndian.Uint32(buf[8:])
		session, ok := s.sessions[ssrc]
		if ok {
			handle = session.HandleRTP
		} else {
			// this is default position for SSRC in RTCP packet
			ssrc = binary.BigEndian.Uint32(buf[4:])
			if session, ok = s.sessions[ssrc]; !ok {
				continue // skip unknown ssrc
			}

			handle = session.HandleRTCP
		}

		if session.Write == nil {
			session.Write = func(b []byte) (int, error) {
				return conn.WriteTo(b, addr)
			}
		}

		atomic.AddUint32(&session.Recv, uint32(n))

		if err = handle(buf[:n]); err != nil {
			return err
		}
	}
}
