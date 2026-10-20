package reolink

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/creds"
)

type source struct {
	config        baichuan.Config
	channel       uint8
	stream        baichuan.Stream
	remote        string
	backchannel   bool
	suppressVideo bool
	suppressAudio bool
	username      string
	password      string
	identity      string
	protocol      string
}

func (s source) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "reolink.source{Remote:%q, Channel:%d, Stream:%q, Backchannel:%t}",
		s.remote, s.channel, s.stream, s.backchannel)
}

func parseURL(rawURL string) (source, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return source{}, fmt.Errorf("reolink: invalid source URL")
	}
	if u.Scheme != "reolink" || u.Hostname() == "" || u.Fragment != "" || u.User == nil || u.User.Username() == "" {
		return source{}, fmt.Errorf("reolink: invalid source")
	}
	username := u.User.Username()
	password, _ := u.User.Password()
	creds.AddSecret(username)
	creds.AddSecret(password)

	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return source{}, fmt.Errorf("reolink: invalid query")
	}
	if query.Get("transport") == "uid" {
		creds.AddSecret(u.Hostname())
	}
	for key, values := range query {
		if key != "channel" && key != "stream" && key != "backchannel" && key != "transport" &&
			key != "local" && key != "broadcast" && key != "video" && key != "audio" || len(values) != 1 {
			return source{}, fmt.Errorf("reolink: invalid query parameter")
		}
	}

	s := source{
		username: username, password: password,
		stream: baichuan.StreamMain, backchannel: true,
	}
	switch query.Get("transport") {
	case "", "tcp":
		if query.Get("local") != "" || query.Get("broadcast") != "" {
			return source{}, fmt.Errorf("reolink: discovery addresses require UID transport")
		}
		host := canonicalHost(u.Hostname())
		s.config = baichuan.NewConfig(host, username, password)
		if value := u.Port(); value != "" {
			port, err := strconv.ParseUint(value, 10, 16)
			if err != nil || port == 0 {
				return source{}, fmt.Errorf("reolink: invalid port")
			}
			s.config.Port = uint16(port)
		}
		port := s.config.Port
		if port == 0 {
			port = baichuan.DefaultPort
			s.config.Port = port
		}
		s.remote = net.JoinHostPort(host, strconv.Itoa(int(port)))
		s.protocol = "tcp"
	case "uid":
		if u.Port() != "" {
			return source{}, fmt.Errorf("reolink: UID transport does not use a port")
		}
		uid := u.Hostname()
		s.config = baichuan.NewUIDConfig(uid, username, password)
		s.config.UIDLocalAddr = query.Get("local")
		if s.config.UIDLocalAddr != "" {
			address, err := netip.ParseAddr(s.config.UIDLocalAddr)
			if err != nil || !address.Is4() || address.IsLoopback() ||
				!address.IsGlobalUnicast() && !address.IsLinkLocalUnicast() {
				return source{}, fmt.Errorf("reolink: invalid local address")
			}
			s.config.UIDLocalAddr = address.String()
		}
		s.config.UIDBroadcastAddr = query.Get("broadcast")
		if s.config.UIDBroadcastAddr != "" {
			address, err := netip.ParseAddr(s.config.UIDBroadcastAddr)
			if err != nil || !address.Is4() || address.IsUnspecified() ||
				address.IsLoopback() || address.IsMulticast() {
				return source{}, fmt.Errorf("reolink: invalid broadcast address")
			}
			s.config.UIDBroadcastAddr = address.String()
		}
		s.identity = uid
		s.remote = "uid"
		s.protocol = "udp"
	default:
		return source{}, fmt.Errorf("reolink: invalid transport")
	}
	channel, err := strconv.ParseUint(defaultValue(query.Get("channel"), "0"), 10, 8)
	if err != nil {
		return source{}, fmt.Errorf("reolink: invalid channel")
	}
	s.channel = uint8(channel)

	path := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(path, "/") || path != "" && query.Get("stream") != "" {
		return source{}, fmt.Errorf("reolink: invalid stream")
	}
	name := query.Get("stream")
	if path != "" {
		name = path
	}
	s.stream, err = parseStream(name)
	if err != nil {
		return source{}, err
	}
	switch query.Get("backchannel") {
	case "", "1":
	case "0":
		s.backchannel = false
	default:
		return source{}, fmt.Errorf("reolink: invalid backchannel")
	}
	var enabled bool
	if enabled, err = parseEnabled(query.Get("video")); err != nil {
		return source{}, fmt.Errorf("reolink: invalid video")
	}
	s.suppressVideo = !enabled
	if enabled, err = parseEnabled(query.Get("audio")); err != nil {
		return source{}, fmt.Errorf("reolink: invalid audio")
	}
	s.suppressAudio = !enabled
	if s.suppressVideo && s.suppressAudio && !s.backchannel {
		return source{}, fmt.Errorf("reolink: source has no enabled media")
	}

	return s, nil
}

func parseEnabled(value string) (bool, error) {
	switch value {
	case "", "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

func canonicalHost(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func parseStream(value string) (baichuan.Stream, error) {
	switch strings.ToLower(value) {
	case "", "main", strings.ToLower(string(baichuan.StreamMain)):
		return baichuan.StreamMain, nil
	case "sub", strings.ToLower(string(baichuan.StreamSub)):
		return baichuan.StreamSub, nil
	case "extern", "ext", strings.ToLower(string(baichuan.StreamExtern)):
		return baichuan.StreamExtern, nil
	default:
		return "", fmt.Errorf("reolink: invalid stream")
	}
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
