package webrtc

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/sdp/v3"
)

// https://github.com/AlexxIT/go2rtc/issues/1600
func crealityClient(url string) (core.Producer, error) {
	pc, err := PeerConnection(true)
	if err != nil {
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/creality"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "http"
	prod.URL = url

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// TODO: return webrtc.SessionDescription
	offer, err := prod.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	body, err := offerToB64(offer)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "plain/text")

	// TODO: change http.DefaultClient settings
	client := http.Client{Timeout: time.Second * 5000}
	defer client.CloseIdleConnections()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	answer, err := answerFromB64(res.Body)
	if err != nil {
		return nil, err
	}

	log.Trace().Msgf("[webrtc] answer:\n%s", answer)

	if answer, err = fixCrealitySDP(answer, offer); err != nil {
		return nil, err
	}

	if err = prod.SetAnswer(answer); err != nil {
		return nil, err
	}

	return prod, nil
}

func offerToB64(sdp string) (io.Reader, error) {
	// JS object
	v := map[string]string{
		"type": "offer",
		"sdp":  sdp,
	}

	// bytes
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// base64, why? who knows...
	s := base64.StdEncoding.EncodeToString(b)

	return strings.NewReader(s), nil
}

func answerFromB64(r io.Reader) (string, error) {
	// base64
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	// bytes
	if b, err = base64.StdEncoding.DecodeString(string(b)); err != nil {
		return "", err
	}

	// JS object
	var v map[string]string
	if err = json.Unmarshal(b, &v); err != nil {
		return "", err
	}

	// string "v=0..."
	return v["sdp"], nil
}

// fixCrealitySDP cleans up the answer SDP the printer sends and, crucially,
// adds back every H264 payload type we ourselves offered: the Creality
// webrtc_local implementation answers with payload types that don't match
// what it actually puts on the wire (https://github.com/AlexxIT/go2rtc/issues/1600
// fixed the case where the device at least echoes one of its own declared
// PTs; some devices - e.g. Creality K1C - send RTP tagged with a payload
// type that never appears anywhere in their own SDP answer, only in our
// offer). A real browser tolerates this because it falls back to matching
// the incoming payload type against its own offered codecs; we replicate
// that here at the SDP level so pion's codec table already has an entry
// for whatever payload type the camera actually uses.
func fixCrealitySDP(value, offerValue string) (string, error) {
	var sd sdp.SessionDescription
	if err := sd.UnmarshalString(value); err != nil {
		return "", err
	}

	md := sd.MediaDescriptions[0]

	// important to skip first codec, because second codec will be used
	skip := md.MediaName.Formats[0]
	md.MediaName.Formats = md.MediaName.Formats[1:]

	attrs := make([]sdp.Attribute, 0, len(md.Attributes))
	for _, attr := range md.Attributes {
		switch attr.Key {
		case "fmtp", "rtpmap":
			// important to skip fmtp with x-google, because this is second fmtp for same codec
			// and pion library will fail parsing this SDP
			if strings.HasPrefix(attr.Value, skip) || strings.Contains(attr.Value, "x-google") {
				continue
			}
		}
		attrs = append(attrs, attr)
	}

	// known/declared payload types after the cleanup above
	known := map[string]bool{}
	for _, f := range md.MediaName.Formats {
		known[f] = true
	}

	// pull every H264 payload type we offered and add the ones we don't
	// already know about, so pion can still match the real stream even if
	// it uses a payload type the device never declared in its answer
	if offerMD := firstVideoMedia(offerValue); offerMD != nil {
		rtpmaps := map[string]sdp.Attribute{}
		fmtps := map[string]sdp.Attribute{}
		for _, attr := range offerMD.Attributes {
			switch attr.Key {
			case "rtpmap":
				if pt, _, ok := strings.Cut(attr.Value, " "); ok {
					rtpmaps[pt] = attr
				}
			case "fmtp":
				if pt, _, ok := strings.Cut(attr.Value, " "); ok {
					fmtps[pt] = attr
				}
			}
		}

		for _, pt := range offerMD.MediaName.Formats {
			if known[pt] {
				continue
			}
			rtpmap, ok := rtpmaps[pt]
			if !ok || !strings.Contains(strings.ToUpper(rtpmap.Value), "H264") {
				continue
			}
			known[pt] = true
			md.MediaName.Formats = append(md.MediaName.Formats, pt)
			attrs = append(attrs, rtpmap)
			if fmtp, ok := fmtps[pt]; ok {
				attrs = append(attrs, fmtp)
			}
		}
	}

	md.Attributes = attrs

	b, err := sd.Marshal()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func firstVideoMedia(value string) *sdp.MediaDescription {
	var sd sdp.SessionDescription
	if err := sd.UnmarshalString(value); err != nil {
		return nil
	}
	for _, md := range sd.MediaDescriptions {
		if md.MediaName.Media == "video" {
			return md
		}
	}
	return nil
}
