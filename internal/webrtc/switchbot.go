package webrtc

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
)

func switchbotClient(rawURL string, query url.Values) (core.Producer, error) {
	return kinesisClient(rawURL, query, "webrtc/switchbot", func(prod *webrtc.Conn, query url.Values) (any, error) {
		medias := []*core.Media{
			{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		}

		offer, err := prod.CreateOffer(medias)
		if err != nil {
			return nil, err
		}

		v := struct {
			Type       string `json:"type"`
			SDP        string `json:"sdp"`
			Resolution int    `json:"resolution"`
			PlayType   int    `json:"play_type"`
		}{
			Type: "offer",
			SDP:  offer,
		}

		switch query.Get("resolution") {
		case "hd":
			v.Resolution = 0
		case "sd":
			v.Resolution = 1
		}

		return v, nil
	})
}
