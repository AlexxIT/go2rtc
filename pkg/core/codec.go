package core

import (
	"encoding/base64"
	"fmt"
	"github.com/pion/sdp/v3"
	"strconv"
	"strings"
	"unicode"
)

type Codec struct {
	Name        string // H264, PCMU, PCMA, opus...
	ClockRate   uint32 // 90000, 8000, 16000...
	Channels    uint16 // 0, 1, 2
	FmtpLine    string
	PayloadType uint8
}

func (c *Codec) String() string {
	s := fmt.Sprintf("%d %s", c.PayloadType, c.Name)
	if c.ClockRate != 0 && c.ClockRate != 90000 {
		s = fmt.Sprintf("%s/%d", s, c.ClockRate)
	}
	if c.Channels > 0 {
		s = fmt.Sprintf("%s/%d", s, c.Channels)
	}
	return s
}

func (c *Codec) Text() string {
	switch c.Name {
	case CodecH264:
		if profile := DecodeH264(c.FmtpLine); profile != "" {
			return "H.264 " + profile
		}
		return c.Name
	}

	s := c.Name
	if c.ClockRate != 0 && c.ClockRate != 90000 {
		s += "/" + strconv.Itoa(int(c.ClockRate))
	}
	if c.Channels > 0 {
		s += "/" + strconv.Itoa(int(c.Channels))
	}
	return s
}

func (c *Codec) IsRTP() bool {
	return c.PayloadType != PayloadTypeRAW
}

func (c *Codec) Clone() *Codec {
	clone := *c
	return &clone
}

func (c *Codec) Match(remote *Codec) bool {
	switch remote.Name {
	case CodecAll, CodecAny:
		return true
	}

	return c.Name == remote.Name &&
		(c.ClockRate == remote.ClockRate || remote.ClockRate == 0) &&
		(c.Channels == remote.Channels || remote.Channels == 0)
}

func UnmarshalCodec(md *sdp.MediaDescription, payloadType string) *Codec {
	c := &Codec{PayloadType: byte(atoi(payloadType))}

	for _, attr := range md.Attributes {
		switch {
		case c.Name == "" && attr.Key == "rtpmap" && strings.HasPrefix(attr.Value, payloadType):
			i := strings.IndexByte(attr.Value, ' ')
			ss := strings.Split(attr.Value[i+1:], "/")

			c.Name = strings.ToUpper(ss[0])
			// fix tailing space: `a=rtpmap:96 H264/90000 `
			c.ClockRate = uint32(atoi(strings.TrimRightFunc(ss[1], unicode.IsSpace)))

			if len(ss) == 3 && ss[2] == "2" {
				c.Channels = 2
			}
		case c.FmtpLine == "" && attr.Key == "fmtp" && strings.HasPrefix(attr.Value, payloadType):
			if i := strings.IndexByte(attr.Value, ' '); i > 0 {
				c.FmtpLine = attr.Value[i+1:]
			}
		}
	}

	if c.Name == "" {
		// https://en.wikipedia.org/wiki/RTP_payload_formats
		switch payloadType {
		case "0":
			c.Name = CodecPCMU
			c.ClockRate = 8000
		case "8":
			c.Name = CodecPCMA
			c.ClockRate = 8000
		case "10":
			c.Name = CodecPCM
			c.ClockRate = 44100
			c.Channels = 2
		case "11":
			c.Name = CodecPCM
			c.ClockRate = 44100
		case "14":
			c.Name = CodecMP3
			c.ClockRate = 90000 // it's not real sample rate
		case "26":
			c.Name = CodecJPEG
			c.ClockRate = 90000
		default:
			c.Name = payloadType
		}
	}

	return c
}

func atoi(s string) (i int) {
	i, _ = strconv.Atoi(s)
	return
}

func DecodeH264(fmtp string) string {
	if ps := Between(fmtp, "sprop-parameter-sets=", ","); ps != "" {
		if sps, _ := base64.StdEncoding.DecodeString(ps); len(sps) >= 4 {
			var profile string
			switch sps[1] {
			case 0x42:
				profile = "Baseline"
			case 0x4D:
				profile = "Main"
			case 0x58:
				profile = "Extended"
			case 0x64:
				profile = "High"
			default:
				profile = fmt.Sprintf("0x%02X", sps[1])
			}

			return fmt.Sprintf("%s %d.%d", profile, sps[3]/10, sps[3]%10)
		}
	}
	return ""
}
