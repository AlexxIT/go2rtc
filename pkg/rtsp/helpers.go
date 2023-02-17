package rtsp

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"net/url"
	"regexp"
	"strconv"
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
	// fix bug from Reolink Doorbell
	if i := bytes.Index(rawSDP, []byte("a=sendonlym=")); i > 0 {
		rawSDP = append(rawSDP[:i+11], rawSDP[i+10:]...)
		rawSDP[i+10] = '\n'
	}

	// fix bug from Ezviz C6N
	if i := bytes.Index(rawSDP, []byte("H265/90000\r\na=fmtp:96 profile-level-id=420029;")); i > 0 {
		rawSDP[i+3] = '4'
	}

	sd := &sdp.SessionDescription{}
	if err := sd.Unmarshal(rawSDP); err != nil {
		// fix multiple `s=` https://github.com/AlexxIT/WebRTC/issues/417
		re, _ := regexp.Compile("\ns=[^\n]+")
		rawSDP = re.ReplaceAll(rawSDP, nil)

		// fix SDP header for some cameras
		if i := bytes.Index(rawSDP, []byte("\nm=")); i > 0 {
			rawSDP = append([]byte(sdpHeader), rawSDP[i:]...)
			sd = &sdp.SessionDescription{}
			err = sd.Unmarshal(rawSDP)
		}

		if err != nil {
			return nil, err
		}
	}

	medias := streamer.UnmarshalMedias(sd.MediaDescriptions)

	for _, media := range medias {
		// Check buggy SDP with fmtp for H264 on another track
		// https://github.com/AlexxIT/WebRTC/issues/419
		for _, codec := range media.Codecs {
			if codec.Name == streamer.CodecH264 && codec.FmtpLine == "" {
				codec.FmtpLine = findFmtpLine(codec.PayloadType, sd.MediaDescriptions)
			}
		}

		// fix bug in ONVIF spec
		// https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec-v241.pdf
		switch media.Direction {
		case streamer.DirectionRecvonly, "":
			media.Direction = streamer.DirectionSendonly
		case streamer.DirectionSendonly:
			media.Direction = streamer.DirectionRecvonly
		}
	}

	return medias, nil
}

func findFmtpLine(payloadType uint8, descriptions []*sdp.MediaDescription) string {
	s := strconv.Itoa(int(payloadType))
	for _, md := range descriptions {
		codec := streamer.UnmarshalCodec(md, s)
		if codec.FmtpLine != "" {
			return codec.FmtpLine
		}
	}
	return ""
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
