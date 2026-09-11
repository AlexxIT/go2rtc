package onvif

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type DiscoveryDevice struct {
	URL      string
	Name     string
	Hardware string
}

func FindTagValue(b []byte, tag string) string {
	re := regexp.MustCompile(`(?s)<(?:\w+:)?` + tag + `\b[^>]*>([^<]+)`)
	m := re.FindSubmatch(b)
	if len(m) != 2 {
		return ""
	}
	return string(m[1])
}

func FindTagAttribute(b []byte, tag, attribute string) string {
	re := regexp.MustCompile(`(?s)<(?:\w+:)?` + tag + `\b[^>]*\b` + attribute + `="([^"]+)"`)
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

// DiscoveryStreamingDevices return list of tuple (onvif_url, name, hardware)
func DiscoveryStreamingDevices() ([]DiscoveryDevice, error) {
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

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var devices []DiscoveryDevice

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

		device := DiscoveryDevice{
			URL: FindTagValue(b[:n], "XAddrs"),
		}

		if device.URL == "" {
			continue
		}

		// fix some buggy cameras
		// <wsdd:XAddrs>http://0.0.0.0:8080/onvif/device_service</wsdd:XAddrs>
		if s, ok := strings.CutPrefix(device.URL, "http://0.0.0.0"); ok {
			device.URL = "http://" + addr.IP.String() + s
		}

		// try to find the camera name and model (hardware)
		scopes := FindTagValue(b[:n], "Scopes")
		device.Name = findScope(scopes, "onvif://www.onvif.org/name/")
		device.Hardware = findScope(scopes, "onvif://www.onvif.org/hardware/")

		devices = append(devices, device)
	}

	return devices, nil
}

func findScope(s, prefix string) string {
	s = core.Between(s, prefix, " ")
	s, _ = url.QueryUnescape(s)
	return s
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

// GetPosixTZ returns a POSIX-style TZ string suitable for ONVIF
// GetSystemDateAndTime responses. ONVIF (per IEEE 1003.1 POSIX) expects
// strings of the form `std[offset[dst[offset][,start,end]]]`, e.g.
// "UTC0", "EST5EDT,M3.2.0,M11.1.0".
//
// POSIX uses the *reverse* sign convention from ISO 8601: UTC-5 (EST) is
// written "EST5", UTC+9 (JST) is written "JST-9".
//
// We don't try to emit DST rules (Go's tzdata doesn't expose them in an
// easily-renderable form). Clients that need DST awareness should fall
// back to UTCDateTime, which we also include in the response.
func GetPosixTZ(current time.Time) string {
	// In DST, advance past the next transition so we report the standard
	// offset rather than the DST offset — POSIX's std field is the
	// non-DST baseline.
	name, offset := current.Zone()
	if current.IsDST() {
		_, end := current.ZoneBounds()
		stdName, stdOffset := end.Add(time.Hour * 25).Zone()
		name, offset = stdName, stdOffset
	}

	if name == "" {
		name = "GMT"
	}

	// POSIX offset is the negation of the ISO offset (in seconds).
	posixSec := -offset
	if posixSec == 0 {
		return name + "0"
	}

	sign := ""
	if posixSec < 0 {
		sign = "-"
		posixSec = -posixSec
	}

	hours := posixSec / 3600
	mins := (posixSec % 3600) / 60
	if mins == 0 {
		return fmt.Sprintf("%s%s%d", name, sign, hours)
	}
	return fmt.Sprintf("%s%s%d:%02d", name, sign, hours, mins)
}

func GetPath(urlOrPath, defPath string) string {
	if urlOrPath == "" || urlOrPath[0] == '/' {
		return defPath
	}
	u, err := url.Parse(urlOrPath)
	if err != nil {
		return defPath
	}
	return GetPath(u.Path, defPath)
}
