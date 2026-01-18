package mdns

import (
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

	return b.Serve(entries)
}

func (b *Browser) Serve(entries []*ServiceEntry) error {
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

		var res dns.Msg // response
		for _, q := range req.Question {
			if q.Qtype != dns.TypePTR || q.Qclass != dns.ClassINET {
				continue
			}

			if q.Name == ServiceDNSSD {
				AppendDNSSD(&res, b.Service)
			} else if q.Name == b.Service {
				for _, entry := range entries {
					AppendEntry(&res, entry, b.Service, localIP)
				}
			} else if entry, ok := names[q.Name]; ok {
				AppendEntry(&res, entry, b.Service, localIP)
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

func AppendEntry(msg *dns.Msg, entry *ServiceEntry, service string, ip net.IP) {
	ptrName := entry.name() + "." + service
	srvName := entry.name() + ".local."

	msg.Answer = append(
		msg.Answer,
		&dns.PTR{
			Hdr: dns.RR_Header{
				Name:   service,       // _home-assistant._tcp.local.
				Rrtype: dns.TypePTR,   // 12
				Class:  dns.ClassINET, // 1
				Ttl:    4500,
			},
			Ptr: ptrName, // Home\ Assistant._home-assistant._tcp.local.
		},
	)
	msg.Extra = append(
		msg.Extra,
		&dns.TXT{
			Hdr: dns.RR_Header{
				Name:   ptrName,         // Home\ Assistant._home-assistant._tcp.local.
				Rrtype: dns.TypeTXT,     // 16
				Class:  ClassCacheFlush, // 32769
				Ttl:    4500,
			},
			Txt: entry.TXT(),
		},
		&dns.SRV{
			Hdr: dns.RR_Header{
				Name:   ptrName,         // Home\ Assistant._home-assistant._tcp.local.
				Rrtype: dns.TypeSRV,     // 33
				Class:  ClassCacheFlush, // 32769
				Ttl:    120,
			},
			Port:   entry.Port, // 8123
			Target: srvName,    // 963f1fa82b7142809711cebe7c826322.local.
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   srvName,         // 963f1fa82b7142809711cebe7c826322.local.
				Rrtype: dns.TypeA,       // 1
				Class:  ClassCacheFlush, // 32769
				Ttl:    120,
			},
			A: ip,
		},
	)
}
