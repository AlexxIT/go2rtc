package reolink

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseURLOptions(t *testing.T) {
	for _, test := range []struct {
		query string
		want  bool
	}{
		{"", false}, {"options=quickack", true},
		{"transport=tcp&options=quickack:true", true},
		{"options=quickack:1", true}, {"options=quickack:false", false},
		{"options=quickack:0", false},
		{"options=" + url.QueryEscape(" quickack : true "), true},
		{"transport=uid&options=quickack:false", false},
	} {
		t.Run(test.query, func(t *testing.T) {
			s, err := parseURL("reolink://user:pass@ABC1234567890001/main?" + test.query)
			if err != nil || s.config.TCPQuickACK != test.want {
				t.Fatalf("quickack=%t, error=%v", s.config.TCPQuickACK, err)
			}
		})
	}
}

func TestParseURLRejectsInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		value string
		cause string
	}{
		{"", "empty option"}, {",quickack", "empty option"}, {"quickack, ", "empty option"},
		{":true", "empty option"}, {"unknown-sentinel", "unknown option"},
		{"quickack, unknown-sentinel:true", "unknown option"},
		{"quickack,quickack", "duplicate option"},
		{"quickack:true, quickack:false", "duplicate option"},
		{"quickack:", "invalid option value"}, {"quickack::true", "invalid option value"},
		{"quickack:sentinel", "invalid option value"}, {"quickack=true", "unknown option"},
	} {
		t.Run(test.value, func(t *testing.T) {
			_, err := parseURL("reolink://user:pass@camera/main?options=" + url.QueryEscape(test.value))
			if err == nil || !strings.Contains(err.Error(), test.cause) || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	for _, query := range []string{"options=quickack&options=quickack", "transport=uid&options=quickack"} {
		if _, err := parseURL("reolink://user:pass@ABC1234567890001/main?" + query); err == nil {
			t.Fatal("invalid option accepted")
		}
	}
}
