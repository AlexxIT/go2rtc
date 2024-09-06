package webrtc

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	pion "github.com/pion/webrtc/v3"
)

// SessionDescription is used to expose local and remote session descriptions.
type SwitchBotSessionDescription struct {
	Type       string              `json:"type"`
	SDP        string              `json:"sdp"`
	Resolution SwitchBotResolution `json:"resolution"`
	PlayType   int                 `json:"play_type"`
}

func switchbotClient(rawURL string, query url.Values) (core.Producer, error) {
	return kinesisClient(rawURL, query, "webrtc/switchbot", &kinesisClientOpts{
		SessionDescriptionModifier: func(sd *pion.SessionDescription) ([]byte, error) {
			resolution, ok := parseSwitchBotResolution(query.Get("resolution"))
			if !ok {
				resolution = SwitchBotResolutionSD
			}
			json, err := json.Marshal(SwitchBotSessionDescription{
				Type:       sd.Type.String(),
				SDP:        sd.SDP,
				Resolution: resolution,
				PlayType:   0,
			})
			return json, err
		},
		MediaModifier: func() ([]*core.Media, error) {
			return []*core.Media{
				{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
				//{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
				//{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
				//{Kind: "Data", Direction: core.DirectionSendRecv},
			}, nil
		},
	})
}

type SwitchBotResolution int

const (
	SwitchBotResolutionHD SwitchBotResolution = 0
	SwitchBotResolutionSD                     = 1
)

func parseSwitchBotResolution(str string) (SwitchBotResolution, bool) {
	var (
		resolutionMap = map[string]SwitchBotResolution{
			"hd": SwitchBotResolutionHD,
			"sd": SwitchBotResolutionSD,
		}
	)
	c, ok := resolutionMap[strings.ToLower(str)]
	return c, ok
}
