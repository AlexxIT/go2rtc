package xnet

import (
	"net"
	"strconv"
)

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
