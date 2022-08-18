package tcp

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"net"
)

type Server struct {
	streamer.Element

	listener net.Listener
	closed   bool
}

func NewServer(address string) (srv *Server, err error) {
	srv = &Server{}
	srv.listener, err = net.Listen("tcp", address)
	return
}

func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			s.Fire(conn)
			_ = conn.Close()
		}()
	}
}

func (s *Server) Close() error {
	return s.listener.Close()
}
