package reolink

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/creds"
)

func TestParseURL(t *testing.T) {
	t.Run("routes", testURLRoutes)
	t.Run("credentials", testURLCredentials)
	t.Run("options", testURLOptions)
	t.Run("invalid", testInvalidURLs)
}

func testURLRoutes(t *testing.T) {
	type route struct {
		host, remote, protocol string
		local, broadcast       string
		port                   uint16
		channel                uint8
		stream                 baichuan.Stream
		backchannel            bool
	}
	const uid = "AbC1234567890XYZ"
	for _, test := range []struct {
		name, rawURL string
		want         route
		secrets      []string
	}{
		{
			"LAN", "reolink://admin:p%40ss@[fd00::1]:9001/sub?channel=2&backchannel=0",
			route{host: "fd00::1", remote: "[fd00::1]:9001", protocol: "tcp", port: 9001, channel: 2, stream: baichuan.StreamSub},
			[]string{"admin", "p@ss"},
		},
		{
			"UID", "reolink://admin:secret@" + uid + "/sub?transport=uid&channel=1&backchannel=0&local=192.0.2.28&broadcast=198.51.100.255",
			route{remote: "uid", protocol: "udp", local: "192.0.2.28", broadcast: "198.51.100.255", channel: 1, stream: baichuan.StreamSub},
			[]string{uid, "secret"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := parseURL(test.rawURL)
			if err != nil {
				t.Fatal(err)
			}
			got := route{
				host: s.config.Host, remote: s.remote, protocol: s.protocol,
				local: s.config.UIDLocalAddr, broadcast: s.config.UIDBroadcastAddr,
				port: s.config.Port, channel: s.channel, stream: s.stream, backchannel: s.backchannel,
			}
			if got != test.want {
				t.Fatalf("route = %+v, want %+v", got, test.want)
			}
			for _, value := range []any{s, s.config} {
				for _, secret := range test.secrets {
					if strings.Contains(fmt.Sprint(value), secret) {
						t.Fatalf("credentials exposed by formatted %T", value)
					}
				}
			}
		})
	}
}

func testURLCredentials(t *testing.T) {
	const username = "bxz-54713896"
	const password = "964380127648519627"
	const raw = "reolink://" + username + ":" + password + "@camera.local/main"
	maskedURL := creds.SecretString("reolink://***@camera.local/main")
	if _, err := parseURL(raw); err != nil {
		t.Fatal(err)
	}
	if creds.SecretString(username) != username {
		t.Fatal("source redaction masked the username outside URL userinfo")
	}
	if creds.SecretString(raw) != maskedURL || creds.SecretString(password) != "***" {
		t.Fatal("source redaction must mask URL credentials and the password")
	}
}

func testURLOptions(t *testing.T) {
	type settings struct {
		quickACK, noRefetch bool
		noVideo, noAudio    bool
		shared              bool
		group               uint8
	}
	const base = "reolink://user:pass@ABC1234567890001/main"
	for _, test := range []struct {
		name, suffix string
		want         settings
	}{
		{"defaults", "", settings{}},
		{"empty_query", "?", settings{}},
		{"quickack_bare", "?options=quickack", settings{quickACK: true}},
		{"quickack_true", "?transport=tcp&options=quickack:true", settings{quickACK: true}},
		{"quickack_one", "?options=quickack:1", settings{quickACK: true}},
		{"quickack_false", "?options=quickack:false", settings{}},
		{"quickack_zero", "?options=quickack:0", settings{}},
		{"quickack_spaces", "?options=" + url.QueryEscape(" quickack : true "), settings{quickACK: true}},
		{"quickack_off_uid", "?transport=uid&options=quickack:false", settings{noRefetch: true}},
		{"shared_bare", "?options=sharedtcp", settings{shared: true}},
		{"combined_options", "?options=quickack,sharedtcp:true,refetch:false", settings{quickACK: true, noRefetch: true, shared: true}},
		{"shared_group_zero", "?options=sharedtcp:0", settings{shared: true}},
		{"shared_group_one", "?options=sharedtcp:1", settings{shared: true, group: 1}},
		{"shared_group_max", "?options=sharedtcp:255", settings{shared: true, group: 255}},
		{"shared_false", "?options=sharedtcp:false", settings{}},
		{"shared_off_uid", "?transport=uid&options=sharedtcp:false", settings{noRefetch: true}},
		{"refetch_disabled", "?options=refetch:false,quickack:true", settings{quickACK: true, noRefetch: true}},
		{"refetch_enabled", "?options=refetch:true,quickack:false", settings{}},
		{"refetch_uid_default", "?transport=uid", settings{noRefetch: true}},
		{"video_zero", "?video=0", settings{noVideo: true, noRefetch: true}},
		{"video_false", "?video=false", settings{noVideo: true, noRefetch: true}},
		{"video_one", "?video=1", settings{}},
		{"video_true", "?video=true", settings{}},
		{"audio_zero", "?audio=0", settings{noAudio: true}},
		{"audio_false", "?audio=false", settings{noAudio: true}},
		{"audio_one", "?audio=1", settings{}},
		{"audio_true", "?audio=true", settings{}},
		{"audio_only", "?video=0&audio=1", settings{noVideo: true, noRefetch: true}},
		{"backchannel_only", "?video=0&audio=false", settings{noVideo: true, noAudio: true, noRefetch: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := parseURL(base + test.suffix)
			if err != nil {
				t.Fatal(err)
			}
			got := settings{
				quickACK: s.config.TCPQuickACK, noRefetch: !s.refetch,
				noVideo: s.suppressVideo, noAudio: s.suppressAudio,
				shared: s.sharedTCP, group: s.sharedGroup,
			}
			if got != test.want {
				t.Fatalf("settings = %+v, want %+v", got, test.want)
			}
		})
	}
}

func testInvalidURLs(t *testing.T) {
	const base = "reolink://user:pass@camera"
	const main = base + "/main"
	for _, test := range []struct {
		name, rawURL, cause string
	}{
		{"scheme", "http://admin:pass@camera", "invalid source"},
		{"missing_userinfo", "reolink://camera", "invalid source"},
		{"channel_overflow", base + "?channel=256", "invalid channel"},
		{"stream_value", base + "?stream=fluent", "invalid stream"},
		{"path_and_stream", main + "?stream=sub", "invalid stream"},
		{"unknown_query", base + "?unknown=1", "invalid query parameter"},
		{"backchannel_value", base + "?backchannel=yes", "invalid backchannel"},
		{"malformed_query", base + "?channel=%zz", "invalid query"},
		{"uid_port", base + ":9000?transport=uid", "UID transport does not use a port"},
		{"transport_value", base + "?transport=cloud", "invalid transport"},
		{"local_address_tcp", base + "?local=192.0.2.28", "discovery addresses require UID transport"},
		{"local_loopback", base + "?transport=uid&local=127.0.0.1", "invalid local address"},
		{"broadcast_address_tcp", base + "?broadcast=198.51.100.255", "discovery addresses require UID transport"},
		{"broadcast_loopback", base + "?transport=uid&broadcast=127.0.0.1", "invalid broadcast address"},
		{"broadcast_multicast", base + "?transport=uid&broadcast=224.0.0.1", "invalid broadcast address"},
		{"no_tracks", main + "?video=0&audio=0&backchannel=0", "source has no enabled media"},
		{"video_value", main + "?video=2", "invalid video"},
		{"audio_value", main + "?audio=yes", "invalid audio"},
		{"duplicate_video", main + "?video=0&video=1", "invalid query parameter"},
		{"duplicate_options", main + "?options=quickack&options=quickack", "invalid query parameter"},
		{"quickack_uid", main + "?transport=uid&options=quickack", "quickack requires TCP transport"},
		{"refetch_without_video", main + "?video=0&options=refetch", "refetch requires TCP video"},
		{"refetch_uid", main + "?transport=uid&options=refetch", "refetch requires TCP video"},
		{"duplicate_refetch", main + "?options=refetch,refetch:false", "duplicate option"},
		{"shared_negative_group", main + "?options=sharedtcp:-1", "invalid option value"},
		{"shared_group_overflow", main + "?options=sharedtcp:256", "invalid option value"},
		{"shared_empty_value", main + "?options=sharedtcp:", "invalid option value"},
		{"duplicate_shared", main + "?options=sharedtcp:0,sharedtcp:1", "duplicate option"},
		{"shared_uid", main + "?transport=uid&options=sharedtcp:0", "sharedtcp requires TCP transport"},
		{"empty_options", main + "?options=", "empty option"},
		{"leading_empty_option", main + "?options=" + url.QueryEscape(",quickack"), "empty option"},
		{"trailing_empty_option", main + "?options=" + url.QueryEscape("quickack, "), "empty option"},
		{"empty_option_name", main + "?options=" + url.QueryEscape(":true"), "empty option"},
		{"unknown_option", main + "?options=unknown-sentinel", "unknown option"},
		{"known_and_unknown_options", main + "?options=" + url.QueryEscape("quickack, unknown-sentinel:true"), "unknown option"},
		{"duplicate_quickack", main + "?options=" + url.QueryEscape("quickack,quickack"), "duplicate option"},
		{"conflicting_quickack", main + "?options=" + url.QueryEscape("quickack:true, quickack:false"), "duplicate option"},
		{"quickack_empty_value", main + "?options=" + url.QueryEscape("quickack:"), "invalid option value"},
		{"quickack_double_colon", main + "?options=" + url.QueryEscape("quickack::true"), "invalid option value"},
		{"quickack_invalid_value", main + "?options=" + url.QueryEscape("quickack:sentinel"), "invalid option value"},
		{"quickack_equals", main + "?options=" + url.QueryEscape("quickack=true"), "unknown option"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := parseURL(test.rawURL)
			if err == nil || !strings.Contains(err.Error(), test.cause) {
				t.Fatalf("error = %v, want %q", err, test.cause)
			}
			if s.sharedTCP || s.sharedGroup != 0 || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("invalid source returned shared=%t group=%d or exposed option text", s.sharedTCP, s.sharedGroup)
			}
		})
	}
}
