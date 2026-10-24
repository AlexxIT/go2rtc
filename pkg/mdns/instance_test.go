package mdns

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// A paired HomeKit controller resolves the instance name it paired with, spaces
// included, with an SRV question that may ask for a unicast response.
func TestInstanceQuestionMatches(t *testing.T) {
	entry := &ServiceEntry{Name: "Hallway 0CCD", IP: net.IPv4(192, 0, 2, 10), Port: 1984, Info: map[string]string{"id": "AA:BB:CC:00:00:01"}}
	key := strings.ToLower(entry.instance() + "." + ServiceHAP)

	q := new(dns.Msg)
	q.SetQuestion(entry.instance()+"."+ServiceHAP, dns.TypeSRV)
	q.Question[0].Qclass = dns.ClassINET | ClassUnicastResponse
	wire, err := q.Pack()
	require.NoError(t, err)
	// On the wire the label is the name itself: no dash, no backslash.
	require.True(t, bytes.Contains(wire, []byte("\x0cHallway 0CCD")))

	var got dns.Msg
	require.NoError(t, got.Unpack(wire))
	require.Equal(t, key, strings.ToLower(got.Question[0].Name))
	require.Equal(t, uint16(dns.ClassINET), got.Question[0].Qclass&^ClassUnicastResponse)

	var res dns.Msg
	AppendEntry(&res, entry, ServiceHAP, entry.IP)
	answer, err := res.Pack()
	require.NoError(t, err)
	require.True(t, bytes.Contains(answer, []byte("\x0cHallway 0CCD")))
}

// The answer states that the host has only an A record, so a controller does
// not wait for an AAAA.
func TestAnswerDeniesAAAA(t *testing.T) {
	entry := &ServiceEntry{Name: "Hallway 0CCD", IP: net.IPv4(192, 0, 2, 10), Port: 1984, Info: map[string]string{"sf": "0"}}
	var res dns.Msg
	AppendEntry(&res, entry, ServiceHAP, entry.IP)
	wire, err := res.Pack()
	require.NoError(t, err)
	var got dns.Msg
	require.NoError(t, got.Unpack(wire))
	var hostNSEC *dns.NSEC
	for _, rr := range got.Extra {
		if n, ok := rr.(*dns.NSEC); ok && n.Hdr.Name == "Hallway-0CCD.local." {
			hostNSEC = n
		}
	}
	require.NotNil(t, hostNSEC)
	require.Equal(t, []uint16{dns.TypeA}, hostNSEC.TypeBitMap)
}

// The host name the SRV record points at stays a valid host label.
func TestHostNameStaysALabel(t *testing.T) {
	entry := &ServiceEntry{Name: "Hallway 0CCD"}
	require.Equal(t, "Hallway-0CCD", entry.name())
}
