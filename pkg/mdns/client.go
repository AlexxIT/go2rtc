package mdns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns" // awesome library for parsing mDNS records
)

const ServiceHAP = "_hap._tcp.local." // HomeKit Accessory Protocol

type ServiceEntry struct {
	Name string            `json:"name,omitempty"`
	IP   net.IP            `json:"ip,omitempty"`
	Port uint16            `json:"port,omitempty"`
	Info map[string]string `json:"info,omitempty"`
}

func (e *ServiceEntry) String() string {
	b, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (e *ServiceEntry) TXT() []string {
	var txt []string
	for k, v := range e.Info {
		txt = append(txt, k+"="+v)
	}
	return txt
}

func (e *ServiceEntry) Complete() bool {
	return e.IP != nil && e.Port > 0 && e.Info != nil
}

func (e *ServiceEntry) Addr() string {
	return fmt.Sprintf("%s:%d", e.IP, e.Port)
}

func (e *ServiceEntry) Host(service string) string {
	return e.name() + "." + strings.TrimRight(service, ".")
}

func (e *ServiceEntry) name() string {
	b := []byte(e.Name)
	for i, c := range b {
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
			continue
		}
		b[i] = '-'
	}
	return string(b)
}

var MulticastAddr = &net.UDPAddr{
	IP:   net.IP{224, 0, 0, 251},
	Port: 5353,
}

const sendTimeout = time.Millisecond * 505
const respTimeout = time.Second * 3

// BasicDiscovery - default golang Multicast UDP listener.
// Does not work well with multiple interfaces.
func BasicDiscovery(service string, onentry func(*ServiceEntry) bool) error {
	conn, err := net.ListenMulticastUDP("udp4", nil, MulticastAddr)
	if err != nil {
		return err
	}

	b := Browser{
		Service:     service,
		Addr:        MulticastAddr,
		Recv:        conn,
		Sends:       []net.PacketConn{conn},
		RecvTimeout: respTimeout,
		SendTimeout: sendTimeout,
	}

	defer b.Close()

	return b.Browse(onentry)
}

// Discovery - better discovery version. Works well with multiple interfaces.
func Discovery(service string, onentry func(*ServiceEntry) bool) error {
	b := Browser{
		Service:     service,
		Addr:        MulticastAddr,
		RecvTimeout: respTimeout,
		SendTimeout: sendTimeout,
	}

	if err := b.ListenMulticastUDP(); err != nil {
		return err
	}

	defer b.Close()

	return b.Browse(onentry)
}

// Query - direct Discovery request on device IP-address. Works even over VPN.
func Query(host, service string) (entry *ServiceEntry, err error) {
	conn, err := net.ListenPacket("udp4", ":0") // shouldn't use ":5353"
	if err != nil {
		return
	}

	br := Browser{
		Service: service,
		Addr: &net.UDPAddr{
			IP:   net.ParseIP(host),
			Port: 5353,
		},
		Recv:        conn,
		Sends:       []net.PacketConn{conn},
		SendTimeout: time.Millisecond * 255,
		RecvTimeout: time.Second,
	}

	defer br.Close()

	err = br.Browse(func(en *ServiceEntry) bool {
		entry = en
		return true
	})

	return
}

// QueryOrDiscovery - useful if we know previous device host and want
// to update port or any other information. Will work even over VPN.
func QueryOrDiscovery(host, service string, onentry func(*ServiceEntry) bool) error {
	entry, _ := Query(host, service)
	if entry != nil && onentry(entry) {
		return nil
	}

	return Discovery(service, onentry)
}

type Browser struct {
	Service string

	Addr  net.Addr
	Recv  net.PacketConn
	Sends []net.PacketConn

	RecvTimeout time.Duration
	SendTimeout time.Duration
}

// ListenMulticastUDP - creates multiple senders socket (each for IP4 interface).
// And one receiver with multicast membership for each sender.
// Receiver will get multicast responses on senders requests.
func (b *Browser) ListenMulticastUDP() error {
	// 1. Collect IPv4 interfaces
	ip4s, err := InterfacesIP4()
	if err != nil {
		return err
	}

	// 2. Create senders
	lc1 := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 1. Allow multicast UDP to listen concurrently across multiple listeners
				_ = SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}

	ctx := context.Background()

	for _, ip4 := range ip4s {
		conn, err := lc1.ListenPacket(ctx, "udp4", ip4.String()+":5353") // same port important
		if err != nil {
			continue
		}
		b.Sends = append(b.Sends, conn)
	}

	if b.Sends == nil {
		return errors.New("no interfaces for listen")
	}

	// 3. Create receiver
	lc2 := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 1. Allow multicast UDP to listen concurrently across multiple listeners
				_ = SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)

				// 2. Disable loop responses
				_ = SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, 0)

				// 3. Allow receive multicast responses on all this addresses
				mreq := &syscall.IPMreq{
					Multiaddr: [4]byte{224, 0, 0, 251},
				}
				_ = SetsockoptIPMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)

				for _, send := range b.Sends {
					addr := send.LocalAddr().(*net.UDPAddr)
					mreq.Interface = [4]byte(addr.IP.To4())
					_ = SetsockoptIPMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
				}
			})
		},
	}

	b.Recv, err = lc2.ListenPacket(ctx, "udp4", "0.0.0.0:5353")

	return err
}

func (b *Browser) Browse(onentry func(*ServiceEntry) bool) error {
	msg := &dns.Msg{
		Question: []dns.Question{
			{Name: b.Service, Qtype: dns.TypePTR, Qclass: dns.ClassINET},
		},
	}

	query, err := msg.Pack()
	if err != nil {
		return err
	}

	if err = b.Recv.SetDeadline(time.Now().Add(b.RecvTimeout)); err != nil {
		return err
	}

	go func() {
		for {
			for _, send := range b.Sends {
				if _, err := send.WriteTo(query, b.Addr); err != nil {
					return
				}
			}
			time.Sleep(b.SendTimeout)
		}
	}()

	processed := map[string]struct{}{"": {}}

	b2 := make([]byte, 1500)
	for {
		// in the Hass docker network can receive same msg from different address
		n, addr, err := b.Recv.ReadFrom(b2)
		if err != nil {
			break
		}

		if err = msg.Unpack(b2[:n]); err != nil {
			continue
		}

		ptr := GetPTR(msg, b.Service)

		if _, ok := processed[ptr]; ok {
			continue
		}

		ip := addr.(*net.UDPAddr).IP

		for _, entry := range NewServiceEntries(msg, ip) {
			if onentry(entry) {
				return nil
			}
		}

		processed[ptr] = struct{}{}
	}

	return nil
}

func (b *Browser) Close() error {
	if b.Recv != nil {
		_ = b.Recv.Close()
	}
	for _, send := range b.Sends {
		_ = send.Close()
	}
	return nil
}

func GetPTR(msg *dns.Msg, service string) string {
	for _, record := range msg.Answer {
		if ptr, ok := record.(*dns.PTR); ok && ptr.Hdr.Name == service {
			return ptr.Ptr
		}
	}
	return ""
}

func NewServiceEntries(msg *dns.Msg, ip net.IP) (entries []*ServiceEntry) {
	records := make([]dns.RR, 0, len(msg.Answer)+len(msg.Ns)+len(msg.Extra))
	records = append(records, msg.Answer...)
	records = append(records, msg.Ns...)
	records = append(records, msg.Extra...)

	// PTR ptr=SomeName._hap._tcp.local. hdr=_hap._tcp.local.
	// TXT txt=...                       hdr=SomeName._hap._tcp.local.
	// SRV target=SomeName.local.        hdr=SomeName._hap._tcp.local.
	// A   a=192.168.1.123               hdr=SomeName.local.

	for _, record := range records {
		ptr, ok := record.(*dns.PTR)
		if !ok {
			continue
		}

		entry := &ServiceEntry{}

		if i := strings.IndexByte(ptr.Ptr, '.'); i > 0 {
			entry.Name = strings.ReplaceAll(ptr.Ptr[:i], `\ `, " ")
		}

		var txt *dns.TXT
		var srv *dns.SRV
		var a *dns.A

		for _, record = range records {
			if txt, ok = record.(*dns.TXT); ok && txt.Hdr.Name == ptr.Ptr {
				entry.Info = make(map[string]string, len(txt.Txt))
				for _, s := range txt.Txt {
					k, v, _ := strings.Cut(s, "=")
					entry.Info[k] = v
				}
				break
			}
		}

		for _, record = range records {
			if srv, ok = record.(*dns.SRV); ok && srv.Hdr.Name == ptr.Ptr {
				entry.Port = srv.Port

				for _, record = range records {
					if a, ok = record.(*dns.A); ok && a.Hdr.Name == srv.Target {
						// device can send multiple IP addresses (ex. Homebridge)
						// use first IP from the list or same IP from sender
						if entry.IP == nil || ip.Equal(a.A) {
							entry.IP = a.A
						}
					}
				}
				break
			}
		}

		entries = append(entries, entry)
	}

	return
}

func InterfacesIP4() ([]net.IP, error) {
	intfs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ips []net.IP

loop:
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
				if ip := v.IP.To4(); ip != nil {
					ips = append(ips, ip)
					continue loop
				}
			}
		}
	}

	return ips, nil
}
