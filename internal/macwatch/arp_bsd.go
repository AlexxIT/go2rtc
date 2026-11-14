//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package macwatch

import (
	"encoding/hex"
	"net"
	"syscall"

	"golang.org/x/net/route"
)

// systemNeighbors returns ARP table using a route socket.
// Unlike the "arp" tool it doesn't need any additional system services, so it
// works inside containers and services with a minimal environment.
func systemNeighbors() ([]Neighbor, error) {
	rib, err := route.FetchRIB(syscall.AF_UNSPEC, route.RIBTypeRoute, syscall.RTF_LLINFO)
	if err != nil {
		return nil, err
	}

	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, err
	}

	var items []Neighbor

	for _, msg := range msgs {
		rm, ok := msg.(*route.RouteMessage)
		if !ok || rm.Flags&syscall.RTF_LLINFO == 0 {
			continue
		}

		inet, ok := rm.Addrs[0].(*route.Inet4Addr)
		if !ok {
			continue
		}

		link, ok := rm.Addrs[1].(*route.LinkAddr)
		if !ok {
			continue
		}

		mac := NormalizeMAC(hex.EncodeToString(link.Addr))
		if !validMAC(mac) {
			continue
		}

		ip := net.IPv4(inet.IP[0], inet.IP[1], inet.IP[2], inet.IP[3])

		items = append(items, Neighbor{IP: ip.String(), MAC: mac, Iface: link.Name})
	}

	return items, nil
}
