package xnet

import (
	"net"
	"strconv"
	"time"
)

// Listen is like net.Listen, but it retries for a while.
//
// A port can stay busy for a few seconds after a restart of the program, even
// when the previous instance is already gone (sockets in TIME_WAIT state,
// port forwarder that is still shutting down, and so on). Without the retry
// the program would keep running without this listener.
func Listen(network, address string) (net.Listener, error) {
	var err error

	for i := 0; i < 10; i++ {
		var ln net.Listener
		if ln, err = net.Listen(network, address); err == nil {
			return ln, nil
		}

		time.Sleep(time.Millisecond * 500)
	}

	return nil, err
}

// Docker has common docker addresses (class B):
// https://en.wikipedia.org/wiki/Private_network
// - docker0 172.17.0.1/16
// - br-xxxx 172.18.0.1/16
// - hassio  172.30.32.1/23
var Docker = net.IPNet{
	IP:   []byte{172, 16, 0, 0},
	Mask: []byte{255, 240, 0, 0},
}

// ParseUnspecifiedPort will return port if address is unspecified
// ex. ":8555" or "0.0.0.0:8555"
func ParseUnspecifiedPort(address string) int {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0
	}

	if host != "" && host != "0.0.0.0" && host != "[::]" {
		return 0
	}

	i, _ := strconv.Atoi(port)
	return i
}

func IPNets(ipFilter func(ip net.IP) bool) ([]*net.IPNet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var nets []*net.IPNet

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, _ := iface.Addrs() // range on nil slice is OK
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip := v.IP.To4()
				if ip == nil {
					continue
				}
				if ipFilter != nil && !ipFilter(ip) {
					continue
				}
				nets = append(nets, v)
			}
		}
	}

	return nets, nil
}
