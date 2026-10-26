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

	"github.com/AlexxIT/go2rtc/pkg/xnet"
	"github.com/miekg/dns" // awesome library for parsing mDNS records
)

const (
	ServiceDNSSD = "_services._dns-sd._udp.local."
	ServiceHAP   = "_hap._tcp.local." // HomeKit Accessory Protocol
)

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

// MulticastAddrV6 is the IPv6 link-local mDNS group (RFC 6762 §3).
var MulticastAddrV6 = &net.UDPAddr{
	IP:   net.ParseIP("ff02::fb"),
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
	Nets  []*net.IPNet
	Recv  net.PacketConn
	Sends []net.PacketConn

	// IPv6 senders and receiver run alongside the IPv4 path. NetsV6 keeps
	// only the addresses (IPv6 mDNS group is per-link, no subnet match).
	SendsV6 []net.PacketConn
	NetsV6  []net.IP
	RecvV6  net.PacketConn

	RecvTimeout time.Duration
	SendTimeout time.Duration
}

// ListenMulticastUDP - creates multiple senders socket (each for IP4 interface).
// And one receiver with multicast membership for each sender.
// Receiver will get multicast responses on senders requests.
func (b *Browser) ListenMulticastUDP() error {
	// 1. Collect IPv4 interfaces
	nets, err := xnet.IPNets(func(ip net.IP) bool {
		return !xnet.Docker.Contains(ip)
	})
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

	for _, ipn := range nets {
		conn, err := lc1.ListenPacket(ctx, "udp4", ipn.IP.String()+":5353") // same port important
		if err != nil {
			continue
		}
		b.Nets = append(b.Nets, ipn)
		b.Sends = append(b.Sends, conn)
	}

	// V4 receiver is best-effort: if no senders bound or :5353 is busy, fall
	// through to the IPv6 path. Only abort when neither family came up.
	var v4err error
	if b.Sends != nil {
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
		b.Recv, v4err = lc2.ListenPacket(ctx, "udp4", ":5353")
	} else {
		v4err = errors.New("no ipv4 interfaces for listen")
	}

	v6err := b.listenMulticastUDPv6(ctx)

	// If the v4 receiver bind failed but v4 senders did open, those FDs
	// are now orphaned — close them so we don't leak sockets just because
	// the receive side lost a race with another responder.
	if b.Recv == nil && len(b.Sends) > 0 {
		for _, send := range b.Sends {
			_ = send.Close()
		}
		b.Sends = nil
		b.Nets = nil
	}
	// Same for v6: senders without a receiver are useless.
	if b.RecvV6 == nil && len(b.SendsV6) > 0 {
		for _, send := range b.SendsV6 {
			_ = send.Close()
		}
		b.SendsV6 = nil
		b.NetsV6 = nil
	}

	// At least one family must work.
	if b.Recv == nil && b.RecvV6 == nil {
		if v4err != nil && v6err != nil {
			return fmt.Errorf("no interfaces for listen: %w", errors.Join(v4err, v6err))
		}
		if v4err != nil {
			return v4err
		}
		return v6err
	}

	return nil
}

// listenMulticastUDPv6 binds an IPv6 sender on every multicast-capable
// up interface and one [::]:5353 receiver with IPV6_JOIN_GROUP for
// ff02::fb on each sender's interface. Returns an error only when no
// sender opened; in that case b.RecvV6 stays nil and the v4 path is
// unaffected.
func (b *Browser) listenMulticastUDPv6(ctx context.Context) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	lc1 := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				_ = SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if isDockerOrVirtualIface(iface.Name) {
			continue
		}

		ip := pickIPv6(iface)
		if ip == nil {
			continue
		}

		// Bind to the specific link-local IP with the interface zone index
		// so the kernel routes outgoing packets via that NIC.
		addr := &net.UDPAddr{IP: ip, Port: 5353, Zone: iface.Name}
		conn, err := lc1.ListenPacket(ctx, "udp6", addr.String())
		if err != nil {
			continue
		}
		b.SendsV6 = append(b.SendsV6, conn)
		b.NetsV6 = append(b.NetsV6, ip)
	}

	if len(b.SendsV6) == 0 {
		return errors.New("no ipv6 interfaces for listen")
	}

	lc2 := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				_ = SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				_ = SetsockoptInt(fd, syscall.IPPROTO_IPV6, ipv6MulticastLoop, 0)
				for _, conn := range b.SendsV6 {
					addr := conn.LocalAddr().(*net.UDPAddr)
					iface, err := net.InterfaceByName(addr.Zone)
					if err != nil {
						continue
					}
					mreq := &syscall.IPv6Mreq{}
					copy(mreq.Multiaddr[:], MulticastAddrV6.IP.To16())
					mreq.Interface = uint32(iface.Index)
					_ = SetsockoptIPv6Mreq(fd, syscall.IPPROTO_IPV6, ipv6JoinGroup, mreq)
				}
			})
		},
	}

	b.RecvV6, err = lc2.ListenPacket(ctx, "udp6", "[::]:5353")
	return err
}

// isDockerOrVirtualIface returns true for interface names container
// runtimes and VPNs typically create. mDNS responses on those advertise
// addresses LAN clients cannot reach, so the IPv6 path skips them.
func isDockerOrVirtualIface(name string) bool {
	prefixes := []string{"docker", "br-", "veth", "vmbr", "kube", "cni", "flannel", "tun", "tap", "wg"}
	for _, p := range prefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

// pickIPv6 returns one usable IPv6 address from the interface, preferring
// a global address (for AAAA records iOS can reach across subnets) over a
// link-local one. ULA (fd00::/8) counts as global for our purposes.
func pickIPv6(iface net.Interface) net.IP {
	addrs, _ := iface.Addrs()
	var linkLocal net.IP
	for _, addr := range addrs {
		ipn, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipn.IP.To4(); ip4 != nil {
			continue // skip v4
		}
		ip := ipn.IP.To16()
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip.IsLinkLocalUnicast() {
			if linkLocal == nil {
				linkLocal = ip
			}
			continue
		}
		// global / ULA / GUA — prefer this
		return ip
	}
	return linkLocal
}

// IPV6_MULTICAST_LOOP and IPV6_JOIN_GROUP option constants — Linux uses 19
// and 20, the BSD/Darwin family uses 11 and 12. The actual constants live
// in the OS-specific syscall_*.go files; the package re-exports them here
// so client.go stays portable.

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

	// Discovery-style consumers always need the IPv4 receiver. The IPv6
	// path is only wired into Server today; if a future caller wants v6
	// browsing it would need to drive RecvV6 in parallel here.
	if b.Recv == nil {
		return errors.New("mdns: ipv4 listener required for browse")
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
	if b.RecvV6 != nil {
		_ = b.RecvV6.Close()
	}
	for _, send := range b.Sends {
		_ = send.Close()
	}
	for _, send := range b.SendsV6 {
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
