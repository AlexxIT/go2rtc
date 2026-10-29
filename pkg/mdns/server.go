package mdns

import (
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// ClassCacheFlush https://datatracker.ietf.org/doc/html/rfc6762#section-10.2
const ClassCacheFlush = 0x8001

// ClassUnicastResponse is the QU bit a question sets in its class field
// https://datatracker.ietf.org/doc/html/rfc6762#section-5.4
const ClassUnicastResponse = 0x8000

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
		name := entry.instance() + "." + b.Service
		names[strings.ToLower(name)] = entry
		// The host name too: a controller asks for its AAAA before connecting.
		names[strings.ToLower(entry.name()+".local.")] = entry
	}

	// A controller that cached an instance at another address only asks again
	// on its backoff schedule (up to an hour); an announcement reaches it now.
	go b.announce(entries)

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
			if q.Qclass&^ClassUnicastResponse != dns.ClassINET {
				continue
			}

			// Any question about one of our instances is answered, not only a
			// browse: a paired controller resolves the instance it knows (SRV,
			// TXT) and does not browse again.
			if entry, ok := names[strings.ToLower(q.Name)]; ok {
				AppendEntry(&res, entry, b.Service, localIP)
			} else if q.Qtype != dns.TypePTR {
				continue
			} else if q.Name == ServiceDNSSD {
				AppendDNSSD(&res, b.Service)
			} else if q.Name == b.Service {
				for _, entry := range entries {
					AppendEntry(&res, entry, b.Service, localIP)
				}
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

// announce sends every entry unsolicited, twice one second apart, out of each
// interface with that interface's own address (RFC 6762 8.3).
func (b *Browser) announce(entries []*ServiceEntry) {
	for i := 0; i < 2; i++ {
		for j, send := range b.Sends {
			var res dns.Msg
			for _, entry := range entries {
				AppendEntry(&res, entry, b.Service, b.Nets[j].IP)
			}
			res.MsgHdr.Response = true
			res.MsgHdr.Authoritative = true
			if data, err := res.Pack(); err == nil {
				_, _ = send.WriteTo(data, MulticastAddr)
			}
		}
		time.Sleep(time.Second)
	}
}

func AppendEntry(msg *dns.Msg, entry *ServiceEntry, service string, ip net.IP) {
	ptrName := entry.instance() + "." + service
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
		// Negative answers (RFC 6762 6.1): the instance has only TXT and SRV,
		// the host only an A record. Without them a controller waits for the
		// AAAA nobody will send before it dials the A.
		&dns.NSEC{
			Hdr: dns.RR_Header{
				Name:   ptrName,
				Rrtype: dns.TypeNSEC,
				Class:  ClassCacheFlush,
				Ttl:    4500,
			},
			NextDomain: ptrName,
			TypeBitMap: []uint16{dns.TypeTXT, dns.TypeSRV},
		},
		&dns.NSEC{
			Hdr: dns.RR_Header{
				Name:   srvName,
				Rrtype: dns.TypeNSEC,
				Class:  ClassCacheFlush,
				Ttl:    120,
			},
			NextDomain: srvName,
			TypeBitMap: []uint16{dns.TypeA},
		},
	)
}
