package mdns

import (
	"github.com/hashicorp/mdns"
	"net"
)

const HostHeaderTail = "._hap._tcp.local"

func NewServer(name string, port int, ips []net.IP, txt []string) (*mdns.Server, error) {
	if ips == nil || ips[0] == nil {
		ips = LocalIPs()
	}

	// important to set hostName manually with any value and `.local.` tail
	// important to set ips manually
	service, _ := mdns.NewMDNSService(
		name, "_hap._tcp", "", name+".local.", port, ips, txt,
	)

	return mdns.NewServer(&mdns.Config{Zone: service})
}

func LocalIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var ips []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		var addrs []net.Addr
		if addrs, err = iface.Addrs(); err != nil {
			continue
		}
		for _, addr := range addrs {
			switch addr := addr.(type) {
			case *net.IPNet:
				ips = append(ips, addr.IP)
			case *net.IPAddr:
				ips = append(ips, addr.IP)
			}
		}
	}
	return ips
}
