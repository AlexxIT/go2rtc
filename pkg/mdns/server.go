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
	var msg dns.Msg

	buf := make([]byte, 1500)
	for {
		n, addr, err := b.Recv.ReadFrom(buf)
		if err != nil {
			break
		}

		if err = msg.Unpack(buf[:n]); err != nil {
			continue
		}

		if !HasQuestionPTP(&msg, b.Service) {
			continue
		}

		remoteIP := addr.(*net.UDPAddr).IP
		localIP := MatchLocalIP(remoteIP)
		if localIP == nil {
			continue
		}

		answer, err := NewDNSAnswer(entries, b.Service, localIP).Pack()
		if err != nil {
			continue
		}

		for _, send := range b.Sends {
			_, _ = send.WriteTo(answer, MulticastAddr)
		}
	}

	return nil
}

func HasQuestionPTP(msg *dns.Msg, name string) bool {
	for _, q := range msg.Question {
		if q.Qtype == dns.TypePTR && q.Name == name {
			return true
		}
	}
	return false
}

func NewDNSAnswer(entries []*ServiceEntry, service string, ip net.IP) *dns.Msg {
	msg := dns.Msg{
		MsgHdr: dns.MsgHdr{
			Response:      true,
			Authoritative: true,
		},
	}

	for _, entry := range entries {
		ptrName := entry.name() + "." + service
		srvName := entry.name() + ".local."

		msg.Answer = append(
			msg.Answer,
			&dns.PTR{
				Hdr: dns.RR_Header{
					Name:   service,
					Rrtype: dns.TypePTR,
					Class:  dns.ClassINET,
					Ttl:    4500,
				},
				Ptr: ptrName,
			},
		)
		msg.Extra = append(
			msg.Extra,
			&dns.TXT{
				Hdr: dns.RR_Header{
					Name:   ptrName,
					Rrtype: dns.TypeTXT,
					Class:  ClassCacheFlush,
					Ttl:    4500,
				},
				Txt: entry.TXT(),
			},
			&dns.SRV{
				Hdr: dns.RR_Header{
					Name:     ptrName,
					Rrtype:   dns.TypeSRV,
					Class:    ClassCacheFlush,
					Ttl:      120,
					Rdlength: 0,
				},
				Port:   entry.Port,
				Target: srvName,
			},
			&dns.A{
				Hdr: dns.RR_Header{
					Name:     srvName,
					Rrtype:   dns.TypeA,
					Class:    ClassCacheFlush,
					Ttl:      120,
					Rdlength: 0,
				},
				A: ip,
			},
		)
	}

	return &msg
}

func MatchLocalIP(remote net.IP) net.IP {
	intfs, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, intf := range intfs {
		if intf.Flags&net.FlagUp == 0 || intf.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if local := v.IP.To4(); local != nil && v.Contains(remote) {
					return local
				}
			}
		}
	}

	return nil
}
