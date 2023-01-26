package rtsp

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtcp"
	"net/url"
	"strings"
)

type RTCP struct {
	Channel byte
	Header  rtcp.Header
	Packets []rtcp.Packet
}

const sdpHeader = `v=0
o=- 0 0 IN IP4 0.0.0.0
s=-
t=0 0`

func UnmarshalSDP(rawSDP []byte) ([]*streamer.Media, error) {
	medias, err := streamer.UnmarshalSDP(rawSDP)
	if err != nil {
		// fix SDP header for some cameras
		i := bytes.Index(rawSDP, []byte("\nm="))
		if i > 0 {
			rawSDP = append([]byte(sdpHeader), rawSDP[i:]...)
			medias, err = streamer.UnmarshalSDP(rawSDP)
		}
		if err != nil {
			return nil, err
		}
	}

	// fix bug in ONVIF spec
	// https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec-v241.pdf
	for _, media := range medias {
		switch media.Direction {
		case streamer.DirectionRecvonly, "":
			media.Direction = streamer.DirectionSendonly
		case streamer.DirectionSendonly:
			media.Direction = streamer.DirectionRecvonly
		}
	}

	return medias, nil
}

// urlParse fix bugs:
// 1. Content-Base: rtsp://::ffff:192.168.1.123/onvif/profile.1/
// 2. Content-Base: rtsp://rtsp://turret2-cam.lan:554/stream1/
func urlParse(rawURL string) (*url.URL, error) {
	if strings.HasPrefix(rawURL, "rtsp://rtsp://") {
		rawURL = rawURL[7:]
	}

	u, err := url.Parse(rawURL)
	if err != nil && strings.HasSuffix(err.Error(), "after host") {
		if i1 := strings.Index(rawURL, "://"); i1 > 0 {
			if i2 := strings.IndexByte(rawURL[i1+3:], '/'); i2 > 0 {
				return urlParse(rawURL[:i1+3+i2] + ":" + rawURL[i1+3+i2:])
			}
		}
	}

	return u, err
}
