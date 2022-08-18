package streamer

import (
	"strings"
)

const (
	JSONType       = "type"
	JSONRemoteAddr = "remote_addr"
	JSONUserAgent  = "user_agent"
	JSONReceive    = "receive"
	JSONSend       = "send"
)

// Message - struct for data exchange in Web API
type Message struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

// other

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
