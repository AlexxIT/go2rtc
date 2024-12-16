package webrtc

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

// whepClient - support WebRTC-HTTP Egress Protocol (WHEP)
// ex: http://localhost:1984/api/webrtc?src=camera1
func xtendTuyaWhepClient(url string, query url.Values) (core.Producer, error) {
	// 1. Prepare variables
	api_path := "/api/xtend_tuya/"
	api_service_sdp_exchange := "webrtc_sdp_exchange"
	api_service_get_ice_servers := "webrtc_get_ice_servers"
	device_id := query.Get("device_id")
	auth_token := query.Get("auth_token")
	channel := query.Get("channel")
	session_id := device_id + strconv.FormatInt(time.Now().UTC().Unix(), 10)

	// 2. Get ICE servers from HA
	conf := pion.Configuration{}
	var err error

	completeUrl := url + api_path + api_service_get_ice_servers + "?device_id=" + device_id + "&session_id=" + session_id + "&format=GO2RTC"
	req, err := http.NewRequest("GET", completeUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth_token)

	client := http.Client{Timeout: time.Second * 5000}
	defer client.CloseIdleConnections()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	ice_servers, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(ice_servers))
	if err != nil {
		log.Warn().Err(err).Caller().Send()
	}

	// 3. Create Peer Connection
	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/xtend_tuya"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "http"
	prod.URL = url

	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// 4. Create offer
	offer, err := prod.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	// shorter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
	re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
	offer = re.ReplaceAllString(offer, "")

	// 5. Send offer
	completeUrl = url + api_path + api_service_sdp_exchange + "?device_id=" + device_id + "&session_id=" + session_id + "&channel=" + channel
	req, err = http.NewRequest("POST", completeUrl, strings.NewReader(offer))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", MimeSDP)
	req.Header.Set("Authorization", "Bearer "+auth_token)

	client = http.Client{Timeout: time.Second * 5000}
	defer client.CloseIdleConnections()

	res, err = client.Do(req)
	if err != nil {
		return nil, err
	}

	// 6. Get answer
	answer, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	desc := pion.SessionDescription{
		Type: pion.SDPTypePranswer,
		SDP:  string(answer),
	}
	if err = pc.SetRemoteDescription(desc); err != nil {
		return nil, err
	}

	if err = prod.SetAnswer(string(answer)); err != nil {
		return nil, err
	}

	return prod, nil
}
