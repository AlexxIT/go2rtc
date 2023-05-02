package onvif

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

func FindTagValue(b []byte, tag string) string {
	re := regexp.MustCompile(tag + `[^>]*>([^<]+)`)
	m := re.FindSubmatch(b)
	if len(m) != 2 {
		return ""
	}
	return string(m[1])
}

// UUID - generate something like 44302cbf-0d18-4feb-79b3-33b575263da3
func UUID() string {
	s := core.RandString(32, 16)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func DiscoveryStreamingHosts() ([]string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}

	msg := `<?xml version="1.0" ?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
	<s:Header xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
		<a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
		<a:MessageID>uuid:` + UUID() + `</a:MessageID>
		<a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
	</s:Header>
	<s:Body>
		<d:Probe xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
            <d:Types>tds:Device</d:Types>
			<d:Scopes>onvif://www.onvif.org/Profile/Streaming</d:Scopes>
		</d:Probe>
	</s:Body>
</s:Envelope>`

	addr := &net.UDPAddr{
		IP:   net.IP{239, 255, 255, 250},
		Port: 3702,
	}

	if _, err = conn.WriteTo([]byte(msg), addr); err != nil {
		return nil, err
	}

	if err = conn.SetReadDeadline(time.Now().Add(time.Second * 3)); err != nil {
		return nil, err
	}

	var hosts []string

	b := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFrom(b)
		if err != nil {
			break
		}

		rawURL := FindTagValue(b[:n], "XAddrs")
		if rawURL == "" {
			continue
		}

		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		if u.Scheme != "http" {
			continue
		}

		hosts = append(hosts, u.Host)
	}

	return hosts, nil
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return i
}
