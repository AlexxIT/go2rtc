package macwatch

import (
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Neighbor is a single entry of the OS ARP (neighbour) table.
type Neighbor struct {
	IP    string
	MAC   string
	Iface string
}

// Neighbors returns ARP table of the host in a cross-platform way.
func Neighbors() ([]Neighbor, error) {
	if runtime.GOOS == "linux" {
		// no external tools required
		if b, err := os.ReadFile("/proc/net/arp"); err == nil {
			return ParseProcNetARP(string(b)), nil
		}
	}

	// native OS API
	if items, err := systemNeighbors(); err == nil && len(items) > 0 {
		return items, nil
	}

	// fallback to the system tool
	if runtime.GOOS == "windows" {
		out, err := run(tool("arp"), "-a")
		if err != nil {
			return nil, err
		}
		return ParseArpWindows(out), nil
	}

	// darwin, freebsd, openbsd, netbsd, linux (net-tools)
	out, err := run(tool("arp"), "-an")
	if err != nil {
		return nil, err
	}
	return ParseArpAN(out), nil
}

// NormalizeMAC converts any MAC address format to "aa:bb:cc:dd:ee:ff".
// Supports "aa:bb:cc:dd:ee:ff", "0:e0:4c:68:0:80" (macOS),
// "AA-BB-CC-DD-EE-FF" (Windows), "aabb.ccdd.eeff" (Cisco) and "aabbccddeeff".
// Returns empty string if the input is not a MAC address.
func NormalizeMAC(s string) string {
	// Cisco style
	if !strings.ContainsAny(s, ":-") && strings.Count(s, ".") == 2 {
		return normalizeHexPairs(hexOnly(strings.ReplaceAll(s, ".", "")))
	}

	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' })
	if len(fields) != 6 {
		return normalizeHexPairs(hexOnly(s))
	}

	var b strings.Builder
	b.Grow(17)

	for i, field := range fields {
		hex, ok := lowerHex(field)
		if !ok || len(hex) == 0 || len(hex) > 2 {
			return ""
		}
		if i > 0 {
			b.WriteByte(':')
		}
		if len(hex) == 1 {
			b.WriteByte('0')
		}
		b.WriteString(hex)
	}

	return b.String()
}

// hexOnly returns lowercase 12 chars hex string or empty string.
func hexOnly(s string) string {
	if v, ok := lowerHex(s); ok && len(v) == 12 {
		return v
	}
	return ""
}

// lowerHex returns lowercase string if it contains only hex digits.
func lowerHex(s string) (string, bool) {
	var buf [12]byte

	if len(s) > len(buf) {
		return "", false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9' || c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
			c += 'a' - 'A'
		default:
			return "", false
		}
		buf[i] = c
	}

	return string(buf[:len(s)]), true
}

// normalizeHexPairs converts "aabbccddeeff" to "aa:bb:cc:dd:ee:ff".
func normalizeHexPairs(s string) string {
	if len(s) != 12 {
		return ""
	}

	out := make([]byte, 0, 17)
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, s[i], s[i+1])
	}

	return string(out)
}

// ParseArpAN parses output of "arp -an" (macOS, BSD) and "arp -a" (Linux).
//
//	macOS: ? (192.168.1.5) at 1:2:3:4:5:6 on en0 ifscope [ethernet]
//	Linux: ? (192.168.1.5) at aa:bb:cc:dd:ee:ff [ether] on eth0
func ParseArpAN(s string) []Neighbor {
	var items []Neighbor

	for _, line := range strings.Split(s, "\n") {
		i := strings.IndexByte(line, '(')
		j := strings.IndexByte(line, ')')
		if i < 0 || j < i {
			continue
		}

		ip := net.ParseIP(line[i+1 : j])
		if ip == nil || ip.To4() == nil {
			continue
		}

		fields := strings.Fields(line[j+1:])
		var mac, iface string

		for k, field := range fields {
			if k+1 == len(fields) {
				break
			}
			switch field {
			case "at":
				mac = fields[k+1]
			case "on":
				iface = fields[k+1]
			}
		}

		if mac = NormalizeMAC(mac); !validMAC(mac) {
			continue
		}

		items = append(items, Neighbor{IP: ip.String(), MAC: mac, Iface: iface})
	}

	return items
}

// ParseProcNetARP parses /proc/net/arp (Linux).
func ParseProcNetARP(s string) []Neighbor {
	var items []Neighbor

	for i, line := range strings.Split(s, "\n") {
		if i == 0 {
			continue // header
		}

		fields := strings.Fields(line)
		if len(fields) < 6 || net.ParseIP(fields[0]) == nil {
			continue
		}

		if mac := NormalizeMAC(fields[3]); validMAC(mac) {
			items = append(items, Neighbor{IP: fields[0], MAC: mac, Iface: fields[5]})
		}
	}

	return items
}

// ParseArpWindows parses output of "arp -a" (Windows).
//
//	192.168.1.5           aa-bb-cc-dd-ee-ff     dynamic
func ParseArpWindows(s string) []Neighbor {
	var items []Neighbor

	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := net.ParseIP(fields[0])
		if ip == nil || ip.To4() == nil {
			continue
		}

		if mac := NormalizeMAC(fields[1]); validMAC(mac) {
			items = append(items, Neighbor{IP: ip.String(), MAC: mac})
		}
	}

	return items
}

// validMAC skips empty, zero and multicast (broadcast) MAC addresses.
func validMAC(mac string) bool {
	if len(mac) != 17 || mac == "00:00:00:00:00:00" {
		return false
	}

	b, err := hex.DecodeString(mac[:2])
	return err == nil && b[0]&1 == 0
}

func run(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// tool returns absolute path for system tool, because go2rtc can run
// as a service with a minimal PATH.
func tool(name string) string {
	if name == "arp" {
		for _, path := range []string{
			"/usr/sbin/arp", "/sbin/arp", "/usr/bin/arp",
			"C:/Windows/System32/arp.exe",
		} {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return name
}
