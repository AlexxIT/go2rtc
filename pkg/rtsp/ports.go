package rtsp

import (
	"fmt"
	"net"
	"sync"
)

var mu sync.Mutex

type UDPPortPair struct {
	RTPListener  *net.UDPConn
	RTCPListener *net.UDPConn
	RTPPort      int
	RTCPPort     int
}

func (p *UDPPortPair) Close() {
	if p.RTPListener != nil {
		_ = p.RTPListener.Close()
	}
	if p.RTCPListener != nil {
		_ = p.RTCPListener.Close()
	}
}

func GetUDPPorts(ip net.IP, maxAttempts int) (*UDPPortPair, error) {
	mu.Lock()
	defer mu.Unlock()

	if ip == nil {
		ip = net.IPv4(0, 0, 0, 0)
	}

	for i := 0; i < maxAttempts; i++ {
		// Get a random even port from the OS
		tempListener, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: 0})
		if err != nil {
			continue
		}

		addr := tempListener.LocalAddr().(*net.UDPAddr)
		basePort := addr.Port
		tempListener.Close()

		// 11. RTP over Network and Transport Protocols (https://www.ietf.org/rfc/rfc3550.txt)
		// For UDP and similar protocols,
		// RTP SHOULD use an even destination port number and the corresponding
		// RTCP stream SHOULD use the next higher (odd) destination port number
		if basePort%2 == 1 {
			basePort--
		}

		// Try to bind both ports
		rtpListener, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: basePort})
		if err != nil {
			continue
		}

		rtcpListener, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: basePort + 1})
		if err != nil {
			rtpListener.Close()
			continue
		}

		return &UDPPortPair{
			RTPListener:  rtpListener,
			RTCPListener: rtcpListener,
			RTPPort:      basePort,
			RTCPPort:     basePort + 1,
		}, nil
	}

	return nil, fmt.Errorf("failed to allocate consecutive UDP ports after %d attempts", maxAttempts)
}
