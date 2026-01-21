package udp

import (
	"fmt"
	"net"
)

type UDPServer struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

func NewUDPServer() (*UDPServer, error) {
	addr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	return &UDPServer{
		conn: conn,
		addr: conn.LocalAddr().(*net.UDPAddr),
	}, nil
}

// GetFreePort returns a free UDP port that can be used for listening
func GetFreePort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		return 0, err
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

// IsPortAvailable checks if a UDP port is available for binding
func IsPortAvailable(port int) bool {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (s *UDPServer) Port() int {
	return s.addr.Port
}

func (s *UDPServer) Addr() string {
	return s.addr.String()
}

func (s *UDPServer) ReadFrom(buffer []byte) (int, *net.UDPAddr, error) {
	return s.conn.ReadFromUDP(buffer)
}

func (s *UDPServer) WriteTo(data []byte, addr *net.UDPAddr) (int, error) {
	return s.conn.WriteToUDP(data, addr)
}

func (s *UDPServer) Close() error {
	return s.conn.Close()
}
