package mdns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestBoundEntryOnSecondaryAddress(t *testing.T) {
	b := Browser{Nets: []*net.IPNet{
		{IP: net.ParseIP("192.168.1.10"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("192.168.1.11"), Mask: net.CIDRMask(24, 32)},
	}}
	remote := net.ParseIP("192.168.1.50")
	entry := &ServiceEntry{Name: "Porch", IP: net.ParseIP("192.168.1.11"), Port: 1985}
	require.True(t, b.MatchLocalIP(remote).Equal(net.ParseIP("192.168.1.10")))
	var response dns.Msg
	AppendEntry(&response, entry, ServiceHAP, b.entryIP(entry, remote))
	require.NotEmpty(t, response.Answer)
	for _, rr := range response.Extra {
		if rr, ok := rr.(*dns.A); ok {
			require.True(t, rr.A.Equal(entry.IP))
		}
	}
	var otherNetwork dns.Msg
	AppendEntry(&otherNetwork, entry, ServiceHAP, b.entryIP(entry, net.ParseIP("192.168.2.50")))
	require.Empty(t, otherNetwork.Answer)
	entry.IP = nil
	require.True(t, b.entryIP(entry, remote).Equal(net.ParseIP("192.168.1.10")))
}
