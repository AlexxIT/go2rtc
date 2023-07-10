package mdns

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"strings"
	"time"
)

const ServiceHAP = "_hap._tcp.local." // HomeKit Accessory Protocol

const requestTimeout = time.Millisecond * 505
const responseTimeout = time.Second * 2

type ServiceEntry struct {
	Name string
	IP   net.IP
	Port uint16
	Info map[string]string
}

func (e *ServiceEntry) Complete() bool {
	return e.IP != nil && e.Port > 0 && e.Info != nil
}

func (e *ServiceEntry) Addr() string {
	return fmt.Sprintf("%s:%d", e.IP, e.Port)
}

func Discovery(service string, onentry func(*ServiceEntry) bool) error {
	addr := &net.UDPAddr{
		IP:   net.IP{224, 0, 0, 251},
		Port: 5353,
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return err
	}

	defer conn.Close()

	if err = conn.SetDeadline(time.Now().Add(responseTimeout)); err != nil {
		return err
	}

	msg := &dns.Msg{
		Question: []dns.Question{
			{service, dns.TypePTR, dns.ClassINET},
		},
	}

	b1, err := msg.Pack()
	if err != nil {
		return err
	}

	go func() {
		for {
			if _, err := conn.WriteToUDP(b1, addr); err != nil {
				return
			}
			time.Sleep(requestTimeout)
		}
	}()

	var skipIPs []net.IP

	b2 := make([]byte, 1500)
loop:
	for {
		n, addr, err := conn.ReadFromUDP(b2)
		if err != nil {
			break
		}

		for _, ip := range skipIPs {
			if ip.Equal(addr.IP) {
				continue loop
			}
		}

		if err = msg.Unpack(b2[:n]); err != nil {
			continue
		}

		if !EqualService(msg, service) {
			continue
		}

		if entry := NewServiceEntry(msg); onentry(entry) {
			break
		}

		skipIPs = append(skipIPs, addr.IP)
	}

	return nil
}

func EqualService(msg *dns.Msg, service string) bool {
	for _, rr := range msg.Answer {
		if rr, ok := rr.(*dns.PTR); ok {
			return strings.HasSuffix(rr.Ptr, service)
		}
	}

	return false
}

func NewServiceEntry(msg *dns.Msg) *ServiceEntry {
	entry := &ServiceEntry{}

	records := make([]dns.RR, 0, len(msg.Answer)+len(msg.Ns)+len(msg.Extra))
	records = append(records, msg.Answer...)
	records = append(records, msg.Ns...)
	records = append(records, msg.Extra...)
	for _, record := range records {
		switch record := record.(type) {
		case *dns.PTR:
			if i := strings.IndexByte(record.Ptr, '.'); i > 0 {
				entry.Name = record.Ptr[:i]
			}
		case *dns.A:
			entry.IP = record.A
		case *dns.SRV:
			entry.Port = record.Port
		case *dns.TXT:
			entry.Info = make(map[string]string, len(record.Txt))
			for _, txt := range record.Txt {
				k, v, _ := strings.Cut(txt, "=")
				entry.Info[k] = v
			}
		}
	}

	return entry
}
