package streams

import (
	"encoding/json"
	"fmt"
	"strings"
)

func AppendDOT(dot []byte, stream *Stream) []byte {
	for _, prod := range stream.producers {
		if prod.conn == nil {
			continue
		}
		c, err := marshalConn(prod.conn)
		if err != nil {
			continue
		}
		dot = c.appendDOT(dot, "producer")
	}
	for _, cons := range stream.consumers {
		c, err := marshalConn(cons)
		if err != nil {
			continue
		}
		dot = c.appendDOT(dot, "consumer")
	}
	return dot
}

func marshalConn(v any) (*conn, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var c conn
	if err = json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

const bytesK = "KMGTP"

func humanBytes(i int) string {
	if i < 1000 {
		return fmt.Sprintf("%d B", i)
	}

	f := float64(i) / 1000
	var n uint8
	for f >= 1000 && n < 5 {
		f /= 1000
		n++
	}
	return fmt.Sprintf("%.2f %cB", f, bytesK[n])
}

type node struct {
	ID     uint32         `json:"id"`
	Codec  map[string]any `json:"codec"`
	Parent uint32         `json:"parent"`
	Childs []uint32       `json:"childs"`
	Bytes  int            `json:"bytes"`
	//Packets uint32         `json:"packets"`
	//Drops   uint32         `json:"drops"`
}

var codecKeys = []string{"codec_name", "sample_rate", "channels", "profile", "level"}

func (n *node) name() string {
	if name, ok := n.Codec["codec_name"].(string); ok {
		return name
	}
	return "unknown"
}

func (n *node) codec() []byte {
	b := make([]byte, 0, 128)
	for _, k := range codecKeys {
		if v := n.Codec[k]; v != nil {
			b = fmt.Appendf(b, "%s=%v\n", k, v)
		}
	}
	if l := len(b); l > 0 {
		return b[:l-1]
	}
	return b
}

func (n *node) appendDOT(dot []byte, group string) []byte {
	dot = fmt.Appendf(dot, "%d [group=%s, label=%q, title=%q];\n", n.ID, group, n.name(), n.codec())
	//for _, sink := range n.Childs {
	//	dot = fmt.Appendf(dot, "%d -> %d;\n", n.ID, sink)
	//}
	return dot
}

type conn struct {
	ID         uint32 `json:"id"`
	FormatName string `json:"format_name"`
	Protocol   string `json:"protocol"`
	RemoteAddr string `json:"remote_addr"`
	Source     string `json:"source"`
	URL        string `json:"url"`
	UserAgent  string `json:"user_agent"`
	Receivers  []node `json:"receivers"`
	Senders    []node `json:"senders"`
	BytesRecv  int    `json:"bytes_recv"`
	BytesSend  int    `json:"bytes_send"`
}

func (c *conn) appendDOT(dot []byte, group string) []byte {
	host := c.host()
	dot = fmt.Appendf(dot, "%s [group=host];\n", host)
	dot = fmt.Appendf(dot, "%d [group=%s, label=%q, title=%q];\n", c.ID, group, c.FormatName, c.label())
	if group == "producer" {
		dot = fmt.Appendf(dot, "%s -> %d [label=%q];\n", host, c.ID, humanBytes(c.BytesRecv))
	} else {
		dot = fmt.Appendf(dot, "%d -> %s [label=%q];\n", c.ID, host, humanBytes(c.BytesSend))
	}

	for _, recv := range c.Receivers {
		dot = fmt.Appendf(dot, "%d -> %d [label=%q];\n", c.ID, recv.ID, humanBytes(recv.Bytes))
		dot = recv.appendDOT(dot, "node")
	}
	for _, send := range c.Senders {
		dot = fmt.Appendf(dot, "%d -> %d [label=%q];\n", send.Parent, c.ID, humanBytes(send.Bytes))
		//dot = fmt.Appendf(dot, "%d -> %d [label=%q];\n", send.ID, c.ID, humanBytes(send.Bytes))
		//dot = send.appendDOT(dot, "node")
	}
	return dot
}

func (c *conn) host() (s string) {
	if c.Protocol == "pipe" {
		return "127.0.0.1"
	}

	if s = c.RemoteAddr; s == "" {
		return "unknown"
	}

	if i := strings.Index(s, "forwarded"); i > 0 {
		s = s[i+10:]
	}

	if s[0] == '[' {
		if i := strings.Index(s, "]"); i > 0 {
			return s[1:i]
		}
	}

	if i := strings.IndexAny(s, " ,:"); i > 0 {
		return s[:i]
	}
	return
}

func (c *conn) label() string {
	var sb strings.Builder
	sb.WriteString("format_name=" + c.FormatName)
	if c.Protocol != "" {
		sb.WriteString("\nprotocol=" + c.Protocol)
	}
	if c.Source != "" {
		sb.WriteString("\nsource=" + c.Source)
	}
	if c.URL != "" {
		sb.WriteString("\nurl=" + c.URL)
	}
	if c.UserAgent != "" {
		sb.WriteString("\nuser_agent=" + c.UserAgent)
	}
	return sb.String()
}
