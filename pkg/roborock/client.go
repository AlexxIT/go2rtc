package roborock

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/rpc"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/roborock/iot"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

// Deprecated: should be rewritten to core.Connection
type Client struct {
	core.Listener

	url string

	conn *webrtc.Conn
	iot  *rpc.Client

	devKey   string
	pin      string
	devTopic string

	audio       bool
	backchannel bool
}

func Dial(rawURL string) (*Client, error) {
	client := &Client{url: rawURL}
	if err := client.Dial(); err != nil {
		return nil, err
	}
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) Dial() error {
	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}

	if c.iot, err = iot.Dial(c.url); err != nil {
		return err
	}

	c.pin = u.Query().Get("pin")
	if c.pin != "" {
		c.pin = fmt.Sprintf("%x", md5.Sum([]byte(c.pin)))
		return c.CheckHomesecPassword()
	}

	return nil
}

func (c *Client) Connect() error {
	// 1. Check if camera ready for connection
	for i := 0; ; i++ {
		clientID, err := c.GetHomesecConnectStatus()
		if err != nil {
			return err
		}
		if clientID == "none" {
			break
		}
		if err = c.StopCameraPreview(clientID); err != nil {
			return err
		}
		if i == 5 {
			return errors.New("camera not ready")
		}
		time.Sleep(time.Second)
	}

	// 2. Start camera
	if err := c.StartCameraPreview(); err != nil {
		return err
	}

	// 3. Get TURN config
	conf := pion.Configuration{}

	if turn, _ := c.GetTurnServer(); turn != nil {
		conf.ICEServers = append(conf.ICEServers, *turn)
	}

	// 4. Create Peer Connection
	api, err := webrtc.NewAPI()
	if err != nil {
		return err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return err
	}

	var connected = make(chan bool)
	var sendOffer sync.WaitGroup

	c.conn = webrtc.NewConn(pc)
	c.conn.FormatName = "roborock"
	c.conn.Mode = core.ModeActiveProducer
	c.conn.Protocol = "mqtt"
	c.conn.URL = c.url
	c.conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			if msg != nil && msg.Component == 1 {
				sendOffer.Wait()
				_ = c.SendICEtoRobot(msg.ToJSON().Candidate, "0")
			}
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateConnecting {
				return
			}
			// unblocking write to channel
			select {
			case connected <- msg == pion.PeerConnectionStateConnected:
			default:
			}
		}
	})

	// 5. Send Offer
	sendOffer.Add(1)

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
	}

	if _, err = c.conn.CreateOffer(medias); err != nil {
		return err
	}

	offer := pc.LocalDescription()
	//log.Printf("[roborock] offer\n%s", offer.SDP)
	if err = c.SendSDPtoRobot(offer); err != nil {
		return err
	}

	sendOffer.Done()

	// 6. Receive answer
	ts := time.Now().Add(time.Second * 5)
	for {
		time.Sleep(time.Second)

		if desc, _ := c.GetDeviceSDP(); desc != nil {
			//log.Printf("[roborock] answer\n%s", desc.SDP)
			if err = c.conn.SetAnswer(desc.SDP); err != nil {
				return err
			}
			break
		}

		if time.Now().After(ts) {
			return errors.New("can't get device SDP")
		}
	}

	ticker := time.NewTicker(time.Second * 2)
	for {
		select {
		case <-ticker.C:
			// 7. Receive remote candidates
			if pc.ICEConnectionState() == pion.ICEConnectionStateCompleted {
				ticker.Stop()
				continue
			}

			if ice, _ := c.GetDeviceICE(); ice != nil {
				for _, candidate := range ice {
					_ = c.conn.AddCandidate(candidate)
				}
			}

		case ok := <-connected:
			// 8. Wait connected result (true or false)
			if !ok {
				return errors.New("can't connect")
			}

			return nil
		}
	}
}

func (c *Client) CheckHomesecPassword() (err error) {
	var ok bool

	params := `{"password":"` + c.pin + `"}`
	if err = c.iot.Call("check_homesec_password", params, &ok); err != nil {
		return
	}

	if !ok {
		return errors.New("wrong pin code")
	}

	return nil
}

func (c *Client) GetHomesecConnectStatus() (clientID string, err error) {
	var res []byte

	if err = c.iot.Call("get_homesec_connect_status", nil, &res); err != nil {
		return
	}

	var v struct {
		Status   int    `json:"status"`
		ClientID string `json:"client_id"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return
	}

	return v.ClientID, nil
}

func (c *Client) StartCameraPreview() error {
	params := `{"client_id":"676f32727463","quality":"HD","password":"` + c.pin + `"}`
	return c.Request("start_camera_preview", params)
}

func (c *Client) StopCameraPreview(clientID string) error {
	params := `{"client_id":"` + clientID + `"}`
	return c.Request("stop_camera_preview", params)
}

func (c *Client) GetTurnServer() (turn *pion.ICEServer, err error) {
	var res []byte

	if err = c.iot.Call("get_turn_server", nil, &res); err != nil {
		return
	}

	var v struct {
		URL  string `json:"url"`
		User string `json:"user"`
		Pwd  string `json:"pwd"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return nil, err
	}

	turn = &pion.ICEServer{
		URLs:       []string{v.URL},
		Username:   v.User,
		Credential: v.Pwd,
	}

	return
}

func (c *Client) SendSDPtoRobot(offer *pion.SessionDescription) (err error) {
	b, err := json.Marshal(offer)
	if err != nil {
		return
	}

	params := `{"app_sdp":"` + base64.StdEncoding.EncodeToString(b) + `"}`
	return c.iot.Call("send_sdp_to_robot", params, nil)
}

func (c *Client) SendICEtoRobot(candidate string, mid string) (err error) {
	b := []byte(`{"candidate":"` + candidate + `","sdpMLineIndex":` + mid + `,"sdpMid":"` + mid + `"}`)

	params := `{"app_ice":"` + base64.StdEncoding.EncodeToString(b) + `"}`
	return c.iot.Call("send_ice_to_robot", params, nil)
}

func (c *Client) GetDeviceSDP() (sd *pion.SessionDescription, err error) {
	var res []byte

	if err = c.iot.Call("get_device_sdp", nil, &res); err != nil {
		return
	}

	if string(res) == `{"dev_sdp":"retry"}` {
		return nil, nil
	}

	var v struct {
		SDP []byte `json:"dev_sdp"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return nil, err
	}

	sd = &pion.SessionDescription{}
	if err = json.Unmarshal(v.SDP, sd); err != nil {
		return nil, err
	}

	return
}

func (c *Client) GetDeviceICE() (ice []string, err error) {
	var res []byte

	if err = c.iot.Call("get_device_ice", nil, &res); err != nil {
		return
	}

	if string(res) == `{"dev_ice":"retry"}` {
		return nil, nil
	}

	var v struct {
		ICE [][]byte `json:"dev_ice"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return
	}

	for _, b := range v.ICE {
		init := pion.ICECandidateInit{}
		if err = json.Unmarshal(b, &init); err != nil {
			return
		}
		ice = append(ice, init.Candidate)
	}

	return
}

func (c *Client) StartVoiceChat() error {
	// record - audio from robot, play - audio to robot?
	params := fmt.Sprintf(`{"record":%t,"play":%t}`, c.audio, c.backchannel)
	return c.Request("start_voice_chat", params)
}

func (c *Client) SwitchVideoQuality(hd bool) error {
	if hd {
		return c.Request("switch_video_quality", `{"quality":"HD"}`)
	} else {
		return c.Request("switch_video_quality", `{"quality":"SD"}`)
	}
}

func (c *Client) SetVoiceChatVolume(volume int) error {
	params := `{"volume":` + strconv.Itoa(volume) + `}`
	return c.Request("set_voice_chat_volume", params)
}

func (c *Client) EnableHomesecVoice(enable bool) error {
	if enable {
		return c.Request("enable_homesec_voice", `{"enable":true}`)
	} else {
		return c.Request("enable_homesec_voice", `{"enable":false}`)
	}
}

func (c *Client) Request(method string, args any) (err error) {
	var reply string

	if err = c.iot.Call(method, args, &reply); err != nil {
		return
	}

	if reply != `["ok"]` {
		return errors.New(reply)
	}

	return
}
