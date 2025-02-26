package net2

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
