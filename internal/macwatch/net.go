package macwatch

import (
	"bytes"
	"net"
	"time"
)

// subnetOf returns a local subnet (network) that contains the given IP
// address. Returns nil if the address is not from a directly connected
// network - in this case MAC address can't be resolved by the host.
func subnetOf(ipStr string) *net.IPNet {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return nil
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		n, ok := addr.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.To4() == nil {
			continue
		}

		if n.Contains(ip) {
			return &net.IPNet{IP: n.IP.Mask(n.Mask), Mask: n.Mask}
		}
	}

	return nil
}

// subnetHosts returns all host addresses of the subnet. Returns nil for
// IPv6 and for too big networks (more than /20).
func subnetHosts(n *net.IPNet) []net.IP {
	if n == nil || n.IP.To4() == nil {
		return nil
	}

	ones, bits := n.Mask.Size()
	if bits != 32 || ones < 20 {
		return nil
	}

	ip := n.IP.Mask(n.Mask).To4()

	last := make(net.IP, 4)
	for i := range last {
		last[i] = ip[i] | ^n.Mask[i]
	}

	var hosts []net.IP

	for cur := nextIP(ip); bytes.Compare(cur, last) < 0; cur = nextIP(cur) {
		hosts = append(hosts, cur)
	}

	return hosts
}

// Sweep sends UDP probes to all hosts of the subnet, so the OS will fill
// the ARP table with their MAC addresses. This is the only way to find a new
// IP address of the host when the DHCP lease has changed.
// Returns count of the sent probes.
func Sweep(n *net.IPNet) int {
	hosts := subnetHosts(n)
	if len(hosts) == 0 {
		return 0
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return 0
	}
	defer conn.Close()

	probe := []byte{0}
	addr := &net.UDPAddr{Port: 9}
	count := 0

	for i, host := range hosts {
		addr.IP = host
		if _, err = conn.WriteToUDP(probe, addr); err == nil {
			count++
		}

		// the kernel has a limit of unresolved neighbours, so sending all
		// probes at once will not fill the ARP table for all hosts
		if i%4 == 3 {
			time.Sleep(time.Millisecond * 20)
		}
	}

	return count
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}

	return next
}

// probePort checks that the host is alive on the given TCP port.
// Empty port means no checks required.
func probePort(ip, port string, timeout time.Duration) bool {
	if port == "" {
		return true
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), timeout)
	if err != nil {
		return false
	}

	_ = conn.Close()
	return true
}
