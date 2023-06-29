package onvif

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func FindTagValue(b []byte, tag string) string {
	re := regexp.MustCompile(`<[^/>]*` + tag + `[^>]*>([^<]+)`)
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

func DiscoveryStreamingURLs() ([]string, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	// https://www.onvif.org/wp-content/uploads/2016/12/ONVIF_Feature_Discovery_Specification_16.07.pdf
	// 5.3 Discovery Procedure:
	msg := `<?xml version="1.0" ?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
	<s:Header xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
		<a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
		<a:MessageID>urn:uuid:` + UUID() + `</a:MessageID>
		<a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
	</s:Header>
	<s:Body>
		<d:Probe xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
			<d:Types />
			<d:Scopes />
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

	var urls []string

	b := make([]byte, 8192)
	for {
		n, addr, err := conn.ReadFromUDP(b)
		if err != nil {
			break
		}

		//log.Printf("[onvif] discovery response addr=%s:\n%s", addr, b[:n])

		// ignore printers, etc
		if !strings.Contains(string(b[:n]), "onvif") {
			continue
		}

		url := FindTagValue(b[:n], "XAddrs")
		if url == "" {
			continue
		}

		// fix some buggy cameras
		// <wsdd:XAddrs>http://0.0.0.0:8080/onvif/device_service</wsdd:XAddrs>
		if strings.HasPrefix(url, "http://0.0.0.0") {
			url = "http://" + addr.IP.String() + url[14:]
		}

		urls = append(urls, url)
	}

	return urls, nil
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
