package webrtc

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun/v4"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestFiltersIPFilter(t *testing.T) {
	tests := []struct {
		name    string
		filters *Filters
		ip      string
		want    bool
	}{
		{"nil filters", nil, "192.168.1.10", true},
		{"nil IPs", &Filters{}, "192.168.1.10", true},
		{"empty IPs", &Filters{IPs: []string{}}, "192.168.1.10", false},
		{"matching IPv4", &Filters{IPs: []string{"192.168.1.10"}}, "192.168.1.10", true},
		{"different IPv4", &Filters{IPs: []string{"192.168.1.10"}}, "192.168.1.11", false},
		{"matching IPv6", &Filters{IPs: []string{"::1"}}, "::1", true},
		{"different IPv6", &Filters{IPs: []string{"::1"}}, "2001:db8::1", false},
		{"multiple IPs", &Filters{IPs: []string{"127.0.0.1", "::1"}}, "::1", true},
		{"Docker IP", &Filters{}, "172.17.0.1", true},
		{"explicit Docker IP", &Filters{IPs: []string{"172.17.0.1"}}, "172.17.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.filters.IPFilter(net.ParseIP(tt.ip)))
		})
	}
}

func TestFiltersInterfaceFilter(t *testing.T) {
	tests := []struct {
		name    string
		filters *Filters
		iface   string
		want    bool
	}{
		{"nil filters", nil, "eth0", true},
		{"nil interfaces", &Filters{}, "eth0", true},
		{"empty interfaces", &Filters{Interfaces: []string{}}, "eth0", false},
		{"matching interface", &Filters{Interfaces: []string{"eth0"}}, "eth0", true},
		{"different interface", &Filters{Interfaces: []string{"eth0"}}, "eth1", false},
		{"multiple interfaces", &Filters{Interfaces: []string{"eth0", "eth1"}}, "eth1", true},
		{"Docker interface", &Filters{}, "docker0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.filters.InterfaceFilter(tt.iface))
		})
	}
}

func TestFiltersIncludeLoopback(t *testing.T) {
	require.False(t, (*Filters)(nil).IncludeLoopback())
	require.False(t, (&Filters{}).IncludeLoopback())
	require.True(t, (&Filters{Loopback: true}).IncludeLoopback())
}

func TestFiltersNetwork(t *testing.T) {
	tests := []struct {
		name    string
		filters *Filters
		udp     string
		tcp     string
	}{
		{"nil filters", nil, "udp", "tcp"},
		{"nil networks", &Filters{}, "udp", "tcp"},
		{"empty networks", &Filters{Networks: []string{}}, "", ""},
		{"UDP4", &Filters{Networks: []string{"udp4"}}, "udp4", ""},
		{"UDP6", &Filters{Networks: []string{"udp6"}}, "udp6", ""},
		{"TCP4", &Filters{Networks: []string{"tcp4"}}, "", "tcp4"},
		{"TCP6", &Filters{Networks: []string{"tcp6"}}, "", "tcp6"},
		{"UDP dual stack", &Filters{Networks: []string{"udp4", "udp6"}}, "udp", ""},
		{"TCP dual stack", &Filters{Networks: []string{"tcp4", "tcp6"}}, "", "tcp"},
		{"IPv4 only", &Filters{Networks: []string{"udp4", "tcp4"}}, "udp4", "tcp4"},
		{"IPv6 only", &Filters{Networks: []string{"udp6", "tcp6"}}, "udp6", "tcp6"},
		{"mixed", &Filters{Networks: []string{"udp4", "tcp6"}}, "udp4", "tcp6"},
		{"all", &Filters{Networks: []string{"udp4", "udp6", "tcp4", "tcp6"}}, "udp", "tcp"},
		{"invalid", &Filters{Networks: []string{"invalid"}}, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.udp, tt.filters.Network("udp"))
			require.Equal(t, tt.tcp, tt.filters.Network("tcp"))
		})
	}
}

func testLoopbackInterface(t *testing.T, ip string) string {
	t.Helper()

	ifaces, err := net.Interfaces()
	require.NoError(t, err)

	target := net.ParseIP(ip)

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagLoopback == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		require.NoError(t, err)

		for _, addr := range addrs {
			if parseNetAddr(addr).Equal(target) {
				return iface.Name
			}
		}
	}

	t.Skipf("loopback interface for %s is unavailable", ip)
	return ""
}

func startTestSTUNServer(t *testing.T, network, host, mappedIP string) string {
	t.Helper()

	addr, err := net.ResolveUDPAddr(network, net.JoinHostPort(host, "0"))
	require.NoError(t, err)

	conn, err := net.ListenUDP(network, addr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
	})

	go func() {
		buf := make([]byte, 1500)

		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}

			req := stun.Message{Raw: buf[:n]}
			if req.Decode() != nil || req.Type != stun.BindingRequest {
				continue
			}

			res, err := stun.Build(
				&req,
				stun.BindingSuccess,
				&stun.XORMappedAddress{
					IP:   net.ParseIP(mappedIP),
					Port: remote.Port,
				},
			)
			if err == nil {
				_, _ = conn.WriteToUDP(res.Raw, remote)
			}
		}
	}()

	return "stun:" + conn.LocalAddr().String()
}

func startTestTURNServer(t *testing.T) (string, *atomic.Int32) {
	t.Helper()

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = ln.Close()
	})

	var dials atomic.Int32

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			dials.Add(1)

			go func(conn net.Conn) {
				defer conn.Close()

				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

				var req stun.Message
				if _, err := req.ReadFrom(conn); err != nil {
					return
				}

				res, err := stun.Build(
					&req,
					stun.NewType(req.Type.Method, stun.ClassErrorResponse),
					stun.CodeForbidden,
				)
				if err == nil {
					_, _ = conn.Write(res.Raw)
				}
			}(conn)
		}
	}()

	return "turn:" + ln.Addr().String() + "?transport=tcp", &dials
}

func gatherTestCandidates(
	t *testing.T, filters *Filters, servers ...webrtc.ICEServer,
) []*webrtc.ICECandidate {
	t.Helper()

	api, err := NewServerAPI("", "", filters)
	require.NoError(t, err)

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: servers,
	})
	require.NoError(t, err)

	defer pc.Close()

	var candidates []*webrtc.ICECandidate
	done := make(chan struct{})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candidates = append(candidates, candidate)
		} else {
			close(done)
		}
	})

	_, err = pc.CreateDataChannel("test", nil)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, pc.SetLocalDescription(offer))

	select {
	case <-done:
		return candidates
	case <-time.After(10 * time.Second):
		t.Fatal("ICE gathering timed out")
		return nil
	}
}

func candidatesByType(
	candidates []*webrtc.ICECandidate, typ webrtc.ICECandidateType,
) []*webrtc.ICECandidate {
	var result []*webrtc.ICECandidate

	for _, candidate := range candidates {
		if candidate.Typ == typ {
			result = append(result, candidate)
		}
	}

	return result
}

func TestICEGatheringDefaultFilters(t *testing.T) {
	const mappedIP = "203.0.113.1"

	url := startTestSTUNServer(t, "udp4", "127.0.0.1", mappedIP)

	tests := []struct {
		name    string
		filters *Filters
	}{
		{"nil filters", nil},
		{"empty filters", &Filters{}},
		{"IPv4 only", &Filters{Networks: []string{"udp4"}}},
		{"include loopback", &Filters{
			Loopback: true,
			Networks: []string{"udp4"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := gatherTestCandidates(t, tt.filters, webrtc.ICEServer{
				URLs: []string{url},
			})

			srflx := candidatesByType(candidates, webrtc.ICECandidateTypeSrflx)

			require.Len(t, srflx, 1)
			require.Equal(t, mappedIP, srflx[0].Address)
			require.Equal(t, "0.0.0.0", srflx[0].RelatedAddress)
		})
	}
}

func TestICEGatheringFilters(t *testing.T) {
	const (
		localIP   = "127.0.0.1"
		mappedIP  = "203.0.113.1"
		missingIP = "192.0.2.123"
	)

	iface := testLoopbackInterface(t, localIP)

	// Use a name that does not belong to any local interface.
	missingIface := "go2rtc-test-nonexistent"
	ifaces, err := net.Interfaces()
	require.NoError(t, err)

	for _, itf := range ifaces {
		require.NotEqual(t, missingIface, itf.Name)

		addrs, err := itf.Addrs()
		require.NoError(t, err)

		for _, addr := range addrs {
			require.False(t, parseNetAddr(addr).Equal(net.ParseIP(missingIP)))
		}
	}

	url := startTestSTUNServer(t, "udp4", localIP, mappedIP)

	tests := []struct {
		name       string
		interfaces []string
		ips        []string
		want       bool
	}{
		{
			name: "empty IPs",
			ips:  []string{},
		},
		{
			name:       "empty interfaces",
			interfaces: []string{},
		},
		{
			name: "matching IP",
			ips:  []string{localIP},
			want: true,
		},
		{
			name: "nonexistent IP",
			ips:  []string{missingIP},
		},
		{
			name: "multiple IPs",
			ips:  []string{missingIP, localIP},
			want: true,
		},
		{
			name:       "matching interface",
			interfaces: []string{iface},
			want:       true,
		},
		{
			name:       "nonexistent interface",
			interfaces: []string{missingIface},
		},
		{
			name:       "multiple interfaces",
			interfaces: []string{missingIface, iface},
			want:       true,
		},
		{
			name:       "matching interface and IP",
			interfaces: []string{iface},
			ips:        []string{localIP},
			want:       true,
		},
		{
			name:       "matching interface and nonexistent IP",
			interfaces: []string{iface},
			ips:        []string{missingIP},
		},
		{
			name:       "nonexistent interface and matching IP",
			interfaces: []string{missingIface},
			ips:        []string{localIP},
		},
		{
			name:       "nonexistent interface and IP",
			interfaces: []string{missingIface},
			ips:        []string{missingIP},
		},
		{
			name:       "empty interfaces and matching IP",
			interfaces: []string{},
			ips:        []string{localIP},
		},
		{
			name:       "matching interface and empty IPs",
			interfaces: []string{iface},
			ips:        []string{},
		},
		{
			name:       "empty interfaces and IPs",
			interfaces: []string{},
			ips:        []string{},
		},
		{
			name:       "multiple interfaces and IPs",
			interfaces: []string{missingIface, iface},
			ips:        []string{missingIP, localIP},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := &Filters{
				Loopback:   true,
				Interfaces: tt.interfaces,
				IPs:        tt.ips,
				Networks:   []string{"udp4"},
			}

			candidates := gatherTestCandidates(t, filters, webrtc.ICEServer{
				URLs: []string{url},
			})

			host := candidatesByType(candidates, webrtc.ICECandidateTypeHost)
			srflx := candidatesByType(candidates, webrtc.ICECandidateTypeSrflx)

			if !tt.want {
				require.Empty(t, host)
				require.Empty(t, srflx)
				return
			}

			require.NotEmpty(t, host)

			if tt.ips != nil {
				for _, candidate := range host {
					require.Equal(t, localIP, candidate.Address)
				}

				require.Len(t, srflx, 1)
			}

			require.NotEmpty(t, srflx)

			var found bool

			for _, candidate := range srflx {
				require.Equal(t, mappedIP, candidate.Address)

				if candidate.RelatedAddress == localIP {
					found = true
				}

				require.NotEqual(t, "0.0.0.0", candidate.RelatedAddress)
			}

			require.True(t, found, "missing loopback srflx candidate")
		})
	}
}

func TestICEGatheringLoopback(t *testing.T) {
	const localIP = "127.0.0.1"

	testLoopbackInterface(t, localIP)

	url := startTestSTUNServer(t, "udp4", localIP, "203.0.113.1")

	for _, loopback := range []bool{false, true} {
		name := "disabled"
		if loopback {
			name = "enabled"
		}

		t.Run(name, func(t *testing.T) {
			filters := &Filters{
				Loopback: loopback,
				IPs:      []string{localIP},
				Networks: []string{"udp4"},
			}

			candidates := gatherTestCandidates(t, filters, webrtc.ICEServer{
				URLs: []string{url},
			})

			host := candidatesByType(candidates, webrtc.ICECandidateTypeHost)
			srflx := candidatesByType(candidates, webrtc.ICECandidateTypeSrflx)

			if loopback {
				require.NotEmpty(t, host)
				require.Len(t, srflx, 1)
				require.Equal(t, localIP, srflx[0].RelatedAddress)
			} else {
				require.Empty(t, host)
				require.Empty(t, srflx)
			}
		})
	}
}

func TestICEGatheringTURN(t *testing.T) {
	const localIP = "127.0.0.1"

	iface := testLoopbackInterface(t, localIP)

	tests := []struct {
		name    string
		filters *Filters
		want    int32
	}{
		{
			name: "nil filters",
			want: 1,
		},
		{
			name:    "empty filters",
			filters: &Filters{},
			want:    1,
		},
		{
			name: "default IPv4 filters",
			filters: &Filters{
				Loopback: true,
				Networks: []string{"udp4", "tcp4"},
			},
			want: 1,
		},
		{
			name: "matching IP",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{localIP},
				Networks: []string{"udp4", "tcp4"},
			},
			want: 1,
		},
		{
			name: "matching interface and IP",
			filters: &Filters{
				Loopback:   true,
				Interfaces: []string{iface},
				IPs:        []string{localIP},
				Networks:   []string{"udp4", "tcp4"},
			},
			want: 1,
		},
		{
			name: "empty IPs",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{},
				Networks: []string{"udp4", "tcp4"},
			},
		},
		{
			name: "empty interfaces",
			filters: &Filters{
				Loopback:   true,
				Interfaces: []string{},
				Networks:   []string{"udp4", "tcp4"},
			},
		},
		{
			name: "loopback disabled",
			filters: &Filters{
				IPs:      []string{localIP},
				Networks: []string{"udp4", "tcp4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, dials := startTestTURNServer(t)

			gatherTestCandidates(t, tt.filters, webrtc.ICEServer{
				URLs:       []string{url},
				Username:   "user",
				Credential: "pass",
			})

			require.Equal(t, tt.want, dials.Load())
		})
	}
}

func TestICEGatheringIPv6(t *testing.T) {
	const (
		localIP  = "::1"
		mappedIP = "2001:db8::1"
	)

	iface := testLoopbackInterface(t, localIP)

	conn, err := net.ListenUDP("udp6", &net.UDPAddr{
		IP: net.ParseIP(localIP),
	})
	if err != nil {
		t.Skipf("IPv6 is unavailable: %v", err)
	}
	_ = conn.Close()

	url := startTestSTUNServer(t, "udp6", localIP, mappedIP)

	tests := []struct {
		name    string
		filters *Filters
		related string
	}{
		{
			name: "default IPv6",
			filters: &Filters{
				Loopback: true,
				Networks: []string{"udp6"},
			},
			related: "::",
		},
		{
			name: "default dual stack",
			filters: &Filters{
				Loopback: true,
				Networks: []string{"udp4", "udp6"},
			},
			related: "::",
		},
		{
			// Pion excludes ::1 in isSupportedIPv6Partial.
			name: "explicit IPv6 loopback",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{localIP},
				Networks: []string{"udp6"},
			},
		},
		{
			name: "explicit interface and IPv6 loopback",
			filters: &Filters{
				Loopback:   true,
				Interfaces: []string{iface},
				IPs:        []string{localIP},
				Networks:   []string{"udp6"},
			},
		},
		{
			name: "explicit dual stack loopback",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{"127.0.0.1", localIP},
				Networks: []string{"udp4", "udp6"},
			},
		},
		{
			name: "IPv4 filter on IPv6 network",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{"127.0.0.1"},
				Networks: []string{"udp6"},
			},
		},
		{
			name: "empty IPs",
			filters: &Filters{
				Loopback: true,
				IPs:      []string{},
				Networks: []string{"udp6"},
			},
		},
		{
			name: "empty interfaces",
			filters: &Filters{
				Loopback:   true,
				Interfaces: []string{},
				Networks:   []string{"udp6"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := gatherTestCandidates(t, tt.filters, webrtc.ICEServer{
				URLs: []string{url},
			})

			srflx := candidatesByType(candidates, webrtc.ICECandidateTypeSrflx)

			if tt.related == "" {
				require.Empty(t, srflx)
				return
			}

			require.Len(t, srflx, 1)
			require.Equal(t, mappedIP, srflx[0].Address)
			require.Equal(t, tt.related, srflx[0].RelatedAddress)
		})
	}
}
