package rtsp

import (
	"bytes"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
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

func UnmarshalSDP(rawSDP []byte) ([]*core.Media, error) {
	sd := &sdp.SessionDescription{}
	if err := sd.Unmarshal(rawSDP); err != nil {
		// fix multiple `s=` https://github.com/AlexxIT/WebRTC/issues/417
		rawSDP = regexp.MustCompile("\ns=[^\n]+").ReplaceAll(rawSDP, nil)

		// fix broken `c=` https://github.com/AlexxIT/go2rtc/issues/1426
		rawSDP = regexp.MustCompile("\nc=[^\n]+").ReplaceAll(rawSDP, nil)

		// fix SDP header for some cameras
		if i := bytes.Index(rawSDP, []byte("\nm=")); i > 0 {
			rawSDP = append([]byte(sdpHeader), rawSDP[i:]...)
		}

		// Fix invalid media type (errSDPInvalidValue) caused by
		// some TP-LINK IP camera, e.g. TL-IPC44GW
		for _, b := range regexp.MustCompile("m=[^ ]+ ").FindAll(rawSDP, -1) {
			switch string(b[2 : len(b)-1]) {
			case "audio", "video", "application":
			default:
				rawSDP = bytes.Replace(rawSDP, b, []byte("m=application "), 1)
			}
		}

		if err == io.EOF {
			rawSDP = append(rawSDP, '\n')
		}

		sd = &sdp.SessionDescription{}
		err = sd.Unmarshal(rawSDP)
		if err != nil {
			return nil, err
		}
	}

	// fix buggy camera https://github.com/AlexxIT/go2rtc/issues/771
	forceDirection := sd.Origin.Username == "CV-RTSPHandler"

	var medias []*core.Media

	for _, md := range sd.MediaDescriptions {
		media := core.UnmarshalMedia(md)

		// Check buggy SDP with fmtp for H264 on another track
		// https://github.com/AlexxIT/WebRTC/issues/419
		for _, codec := range media.Codecs {
			switch codec.Name {
			case core.CodecH264:
				if codec.FmtpLine == "" {
					codec.FmtpLine = findFmtpLine(codec.PayloadType, sd.MediaDescriptions)
				}
			case core.CodecOpus:
				// fix OPUS for some cameras https://datatracker.ietf.org/doc/html/rfc7587
				codec.ClockRate = 48000
				codec.Channels = 2
			}
		}

		if media.Direction == "" || forceDirection {
			media.Direction = core.DirectionRecvonly
		}

		medias = append(medias, media)
	}

	return medias, nil
}

func findFmtpLine(payloadType uint8, descriptions []*sdp.MediaDescription) string {
	s := strconv.Itoa(int(payloadType))
	for _, md := range descriptions {
		codec := core.UnmarshalCodec(md, s)
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
