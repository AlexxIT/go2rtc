package mdns

import (
	"errors"
	"net"

	"github.com/miekg/dns"
)

// ClassCacheFlush https://datatracker.ietf.org/doc/html/rfc6762#section-10.2
const ClassCacheFlush = 0x8001

func Serve(service string, entries []*ServiceEntry) error {
	b := Browser{Service: service}

	if err := b.ListenMulticastUDP(); err != nil {
		return err
	}

	// IPv6 receiver, if available, runs in a sibling goroutine and feeds
	// answers back through the same Browser.Serve loop semantics. When
	// only IPv6 came up (e.g. avahi holds IPv4 :5353), run it on the
	// foreground goroutine so the caller still blocks correctly.
	if b.Recv == nil && b.RecvV6 != nil {
		return b.serveV6(entries)
	}
	if b.RecvV6 != nil {
		go func() { _ = b.serveV6(entries) }()
	}

	return b.Serve(entries)
}

// serveV6 mirrors Serve but reads from RecvV6 and answers back via the
// IPv6 multicast group on the SendsV6 senders. AAAA records are emitted
// using the per-interface IPv6 address; A records are added as well when
// the request also matches an IPv4 net (AppendEntry decides based on the
// answerIP family).
func (b *Browser) serveV6(entries []*ServiceEntry) error {
	names := make(map[string]*ServiceEntry, len(entries))
	for _, entry := range entries {
		names[entry.name()+"."+b.Service] = entry
	}

	buf := make([]byte, 1500)
	for {
		n, addr, err := b.RecvV6.ReadFrom(buf)
		if err != nil {
			return nil
		}

		var req dns.Msg
		if err = req.Unpack(buf[:n]); err != nil {
			continue
		}
		if req.Question == nil {
			continue
		}

		// Resolve the local IPv6 used for the answering interface from the
		// remote's zone (link-local) or from the matching SendsV6 entry.
		remote := addr.(*net.UDPAddr)
		localIP := b.matchLocalIPv6(remote)
		if localIP == nil {
			continue
		}
		peerV4 := b.peerV4ForLocalIPv6(localIP)

		var res dns.Msg
		for _, q := range req.Question {
			if q.Qtype != dns.TypePTR || q.Qclass != dns.ClassINET {
				continue
			}
			if q.Name == ServiceDNSSD {
				AppendDNSSD(&res, b.Service)
			} else if q.Name == b.Service {
				for _, entry := range entries {
					AppendEntryDual(&res, entry, b.Service, peerV4, localIP)
				}
			} else if entry, ok := names[q.Name]; ok {
				AppendEntryDual(&res, entry, b.Service, peerV4, localIP)
			}
		}
		if res.Answer == nil {
			continue
		}
		res.MsgHdr.Response = true
		res.MsgHdr.Authoritative = true

		data, err := res.Pack()
		if err != nil {
			continue
		}
		for _, send := range b.SendsV6 {
			_, _ = send.WriteTo(data, MulticastAddrV6)
		}
	}
}

func (b *Browser) Serve(entries []*ServiceEntry) error {
	if b.Recv == nil {
		return errors.New("mdns: ipv4 receiver not initialised")
	}
	names := make(map[string]*ServiceEntry, len(entries))
	for _, entry := range entries {
		name := entry.name() + "." + b.Service
		names[name] = entry
	}

	buf := make([]byte, 1500)
	for {
		n, addr, err := b.Recv.ReadFrom(buf)
		if err != nil {
			break
		}

		var req dns.Msg // request
		if err = req.Unpack(buf[:n]); err != nil {
			continue
		}

		// skip messages without Questions
		if req.Question == nil {
			continue
		}

		remoteIP := addr.(*net.UDPAddr).IP
		localIP := b.MatchLocalIP(remoteIP)

		// skip messages from unknown networks (can be docker network)
		if localIP == nil {
			continue
		}

		// emit AAAA alongside A when the same interface has a v6 too,
		// so iOS HomeKit can fall back across families during streaming
		peerV6 := b.peerV6ForLocalIP(localIP)

		var res dns.Msg // response
		for _, q := range req.Question {
			if q.Qtype != dns.TypePTR || q.Qclass != dns.ClassINET {
				continue
			}

			if q.Name == ServiceDNSSD {
				AppendDNSSD(&res, b.Service)
			} else if q.Name == b.Service {
				for _, entry := range entries {
					AppendEntryDual(&res, entry, b.Service, localIP, peerV6)
				}
			} else if entry, ok := names[q.Name]; ok {
				AppendEntryDual(&res, entry, b.Service, localIP, peerV6)
			}
		}

		if res.Answer == nil {
			continue
		}

		res.MsgHdr.Response = true
		res.MsgHdr.Authoritative = true

		data, err := res.Pack()
		if err != nil {
			continue
		}

		for _, send := range b.Sends {
			_, _ = send.WriteTo(data, MulticastAddr)
		}
	}

	return nil
}

func (b *Browser) MatchLocalIP(remote net.IP) net.IP {
	for _, ipn := range b.Nets {
		if ipn.Contains(remote) {
			return ipn.IP
		}
	}
	return nil
}

// matchLocalIPv6 returns the local IPv6 bound on the interface the
// request was received on, identified by the zone the kernel attached
// to the remote address. Without a zone we cannot pick an interface, so
// we drop instead of guessing.
func (b *Browser) matchLocalIPv6(remote *net.UDPAddr) net.IP {
	if remote.Zone == "" {
		return nil
	}
	for _, send := range b.SendsV6 {
		la := send.LocalAddr().(*net.UDPAddr)
		if la.Zone == remote.Zone {
			return la.IP
		}
	}
	return nil
}

// peerV6ForLocalIP returns the IPv6 address bound on the same interface
// that owns the given IPv4 net, so the dual-stack AppendEntry path can
// emit both A and AAAA. Returns nil when the interface has no usable v6
// or no v6 listener was opened on it.
func (b *Browser) peerV6ForLocalIP(ipv4 net.IP) net.IP {
	if ipv4 == nil {
		return nil
	}
	iface := ifaceForIPv4(ipv4)
	if iface == "" {
		return nil
	}
	for _, send := range b.SendsV6 {
		la := send.LocalAddr().(*net.UDPAddr)
		if la.Zone == iface {
			return la.IP
		}
	}
	return nil
}

// peerV4ForLocalIPv6 is the IPv6→IPv4 inverse of peerV6ForLocalIP. It
// reads the interface address table directly so it can find a usable
// IPv4 even when go2rtc's own IPv4 mDNS bind never succeeded.
func (b *Browser) peerV4ForLocalIPv6(ipv6 net.IP) net.IP {
	if ipv6 == nil {
		return nil
	}
	iface := ifaceForIPv6(ipv6)
	if iface == "" {
		return nil
	}
	target, err := net.InterfaceByName(iface)
	if err != nil {
		return nil
	}
	addrs, _ := target.Addrs()
	for _, addr := range addrs {
		ipn, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipn.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() && !ip4.IsLoopback() {
			return ip4
		}
	}
	return nil
}

func ifaceForIPv4(ip net.IP) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipn, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipn.IP.To4(); ip4 != nil && ip4.Equal(ip) {
				return iface.Name
			}
		}
	}
	return ""
}

func ifaceForIPv6(ip net.IP) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipn, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipn.IP.To4() == nil && ipn.IP.Equal(ip) {
				return iface.Name
			}
		}
	}
	return ""
}

func AppendDNSSD(msg *dns.Msg, service string) {
	msg.Answer = append(
		msg.Answer,
		&dns.PTR{
			Hdr: dns.RR_Header{
				Name:   ServiceDNSSD,  // _services._dns-sd._udp.local.
				Rrtype: dns.TypePTR,   // 12
				Class:  dns.ClassINET, // 1
				Ttl:    4500,
			},
			Ptr: service, // _home-assistant._tcp.local.
		},
	)
}

// AppendEntry adds the PTR/TXT/SRV/A records for the service. The legacy
// signature is preserved; for IPv6 callers, use AppendEntryDual which
// emits both A and AAAA so a query that arrived over IPv6 still gets a
// usable IPv4 fallback.
func AppendEntry(msg *dns.Msg, entry *ServiceEntry, service string, ip net.IP) {
	if ip4 := ip.To4(); ip4 != nil {
		appendEntry(msg, entry, service, ip4, nil)
	} else {
		appendEntry(msg, entry, service, nil, ip)
	}
}

// AppendEntryDual is like AppendEntry but emits both A and AAAA records
// when both addresses are non-nil. Either argument may be nil.
func AppendEntryDual(msg *dns.Msg, entry *ServiceEntry, service string, ipv4, ipv6 net.IP) {
	appendEntry(msg, entry, service, ipv4, ipv6)
}

func appendEntry(msg *dns.Msg, entry *ServiceEntry, service string, ipv4, ipv6 net.IP) {
	ptrName := entry.name() + "." + service
	srvName := entry.name() + ".local."

	msg.Answer = append(msg.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: service, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 4500},
		Ptr: ptrName,
	})
	msg.Extra = append(msg.Extra,
		&dns.TXT{
			Hdr: dns.RR_Header{Name: ptrName, Rrtype: dns.TypeTXT, Class: ClassCacheFlush, Ttl: 4500},
			Txt: entry.TXT(),
		},
		&dns.SRV{
			Hdr:    dns.RR_Header{Name: ptrName, Rrtype: dns.TypeSRV, Class: ClassCacheFlush, Ttl: 120},
			Port:   entry.Port,
			Target: srvName,
		},
	)

	if ip4 := ipv4.To4(); ip4 != nil {
		msg.Extra = append(msg.Extra, &dns.A{
			Hdr: dns.RR_Header{Name: srvName, Rrtype: dns.TypeA, Class: ClassCacheFlush, Ttl: 120},
			A:   ip4,
		})
	}
	if ipv6 != nil && ipv6.To4() == nil {
		msg.Extra = append(msg.Extra, &dns.AAAA{
			Hdr:  dns.RR_Header{Name: srvName, Rrtype: dns.TypeAAAA, Class: ClassCacheFlush, Ttl: 120},
			AAAA: ipv6,
		})
	}
}
