package unifiprotect

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type sourceConfig struct {
	mac     string
	channel string
	audio   bool
}

func parseSource(source string) (sourceConfig, error) {
	u, err := url.Parse(source)
	if err != nil {
		return sourceConfig{}, err
	}
	if u.Scheme != "unifi-protect" {
		return sourceConfig{}, errors.New("unifi-protect: invalid source scheme")
	}
	if u.User != nil || u.Port() != "" || u.Path != "" && u.Path != "/" || u.Fragment != "" {
		return sourceConfig{}, errors.New("unifi-protect: source must contain only a camera MAC and query options")
	}

	mac, err := normalizeMAC(u.Hostname())
	if err != nil {
		return sourceConfig{}, err
	}

	query := u.Query()
	for key := range query {
		if key != "channel" && key != "audio" {
			return sourceConfig{}, fmt.Errorf("unifi-protect: unsupported source option %q", key)
		}
		if len(query[key]) != 1 {
			return sourceConfig{}, fmt.Errorf("unifi-protect: source option %q must be specified once", key)
		}
	}

	channel := query.Get("channel")
	if channel == "" {
		channel = "video1"
	}
	if channel != "video1" && channel != "video2" && channel != "video3" {
		return sourceConfig{}, fmt.Errorf("unifi-protect: unsupported channel %q", channel)
	}

	audio := true
	switch query.Get("audio") {
	case "", "1", "true":
	case "0", "false":
		audio = false
	default:
		return sourceConfig{}, errors.New("unifi-protect: audio must be 0 or 1")
	}

	return sourceConfig{mac: mac, channel: channel, audio: audio}, nil
}

func normalizeMAC(value string) (string, error) {
	value = strings.Map(func(r rune) rune {
		switch r {
		case ':', '-', '.':
			return -1
		}
		return r
	}, value)
	if len(value) != 12 {
		return "", errors.New("unifi-protect: camera MAC must contain 12 hexadecimal digits")
	}
	for _, c := range value {
		if c < '0' || c > '9' && c < 'A' || c > 'F' && c < 'a' || c > 'f' {
			return "", errors.New("unifi-protect: camera MAC must contain 12 hexadecimal digits")
		}
	}
	return strings.ToUpper(value), nil
}
