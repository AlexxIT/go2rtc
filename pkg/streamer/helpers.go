package streamer

import (
	"strings"
	"time"
)

type Info struct {
	Type       string   `json:"type,omitempty"`
	URL        string   `json:"url,omitempty"`
	RemoteAddr string   `json:"remote_addr,omitempty"`
	UserAgent  string   `json:"user_agent,omitempty"`
	Medias     []*Media `json:"medias,omitempty"`
	Tracks     []*Track `json:"tracks,omitempty"`
	Recv       uint32   `json:"recv,omitempty"`
	Send       uint32   `json:"send,omitempty"`
}

func Between(s, sub1, sub2 string) string {
	i := strings.Index(s, sub1)
	if i < 0 {
		return ""
	}
	s = s[i+len(sub1):]

	if len(sub2) == 1 {
		i = strings.IndexByte(s, sub2[0])
	} else {
		i = strings.Index(s, sub2)
	}
	if i >= 0 {
		return s[:i]
	}

	return s
}

func Contains(medias []*Media, media *Media, codec *Codec) bool {
	var ok1, ok2 bool
	for _, m := range medias {
		if m == media {
			ok1 = true
			break
		}
	}
	for _, c := range media.Codecs {
		if c == codec {
			ok2 = true
			break
		}
	}
	return ok1 && ok2
}

type Probe struct {
	deadline time.Time
	items    map[interface{}]struct{}
}

func NewProbe(enable bool) *Probe {
	if enable {
		return &Probe{
			deadline: time.Now().Add(time.Second * 3),
			items:    map[interface{}]struct{}{},
		}
	} else {
		return nil
	}
}

// Active return true if probe enabled and not finish
func (p *Probe) Active() bool {
	return len(p.items) < 2 && time.Now().Before(p.deadline)
}

// Append safe to run if Probe is nil
func (p *Probe) Append(v interface{}) {
	if p != nil {
		p.items[v] = struct{}{}
	}
}
