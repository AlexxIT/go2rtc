package mdns

import (
	"context"
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

	ctx := context.Background()

	// 2. Create senders
	lc1 := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 1. Allow multicast UDP to listen concurrently across multiple listeners
				_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}

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
				_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)

				// 2. Disable loop responses
				_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, 0)

				// 3. Allow receive multicast responses on all this addresses
				mreq := &syscall.IPMreq{
					Multiaddr: [4]byte{224, 0, 0, 251},
				}
				_ = syscall.SetsockoptIPMreq(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)

				for _, send := range b.Sends {
					addr := send.LocalAddr().(*net.UDPAddr)
					mreq.Interface = [4]byte(addr.IP.To4())
					_ = syscall.SetsockoptIPMreq(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
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
			{b.Service, dns.TypePTR, dns.ClassINET},
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

	var skipPTR []string

	b2 := make([]byte, 1500)
loop:
	for {
		// in the Hass docker network can receive same msg from different address
		n, _, err := b.Recv.ReadFrom(b2)
		if err != nil {
			break
		}

		if err = msg.Unpack(b2[:n]); err != nil {
			continue
		}

		ptr := GetPTR(msg)

		if !strings.HasSuffix(ptr, b.Service) {
			continue
		}

		for _, s := range skipPTR {
			if s == ptr {
				continue loop
			}
		}

		if entry := NewServiceEntry(msg); onentry(entry) {
			break
		}

		skipPTR = append(skipPTR, ptr)
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

func GetPTR(msg *dns.Msg) string {
	for _, rr := range msg.Answer {
		if rr, ok := rr.(*dns.PTR); ok {
			return rr.Ptr
		}
	}
	return ""
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
