package tuya

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api        TuyaAPI
	conn       *webrtc.Conn
	pc         *pion.PeerConnection
	dc         *pion.DataChannel
	videoSSRC  uint32
	audioSSRC  uint32
	streamType int
	isHEVC     bool
	connected  core.Waiter
	closed     bool
	handlers   map[uint32]func(*rtp.Packet)
}

type DataChannelMessage struct {
	Type string `json:"type"`
	Msg  string `json:"msg"`
}

type RecvMessage struct {
	Video struct {
		SSRC uint32 `json:"ssrc"`
	} `json:"video"`
	Audio struct {
		SSRC uint32 `json:"ssrc"`
	} `json:"audio"`
}

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	// Tuya Smart API
	email := query.Get("email")
	password := query.Get("password")

	// Tuya Cloud API
	uid := query.Get("uid")
	clientId := query.Get("client_id")
	clientSecret := query.Get("client_secret")

	// Shared params
	deviceId := query.Get("device_id")

	// Stream params
	streamResolution := query.Get("resolution")

	useTuyaApi := deviceId != "" && email != "" && password != ""
	useCloudApi := deviceId != "" && uid != "" && clientId != "" && clientSecret != ""

	if streamResolution == "" || (streamResolution != "hd" && streamResolution != "sd") {
		streamResolution = "hd"
	}

	if !useTuyaApi && !useCloudApi {
		return nil, errors.New("tuya: wrong query params")
	}

	client := &Client{
		handlers: make(map[uint32]func(*rtp.Packet)),
	}

	if useTuyaApi {
		if client.api, err = NewTuyaSmartApiClient(nil, u.Hostname(), email, password, deviceId); err != nil {
			return nil, fmt.Errorf("tuya: %w", err)
		}
	} else {
		if client.api, err = NewTuyaCloudApiClient(u.Hostname(), uid, deviceId, clientId, clientSecret); err != nil {
			return nil, fmt.Errorf("tuya: %w", err)
		}
	}

	if err := client.api.Init(); err != nil {
		return nil, fmt.Errorf("tuya: %w", err)
	}

	client.streamType = client.api.GetStreamType(streamResolution)
	client.isHEVC = client.api.IsHEVC(client.streamType)

	// Create a new PeerConnection
	conf := pion.Configuration{
		ICEServers:         client.api.GetICEServers(),
		ICETransportPolicy: pion.ICETransportPolicyAll,
		BundlePolicy:       pion.BundlePolicyMaxBundle,
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		client.Close(err)
		return nil, err
	}

	client.pc, err = api.NewPeerConnection(conf)
	if err != nil {
		client.Close(err)
		return nil, err
	}

	// protect from sending ICE candidate before Offer
	var sendOffer core.Waiter

	// protect from blocking on errors
	defer sendOffer.Done(nil)

	// Create new WebRTC connection
	client.conn = webrtc.NewConn(client.pc)
	client.conn.FormatName = "tuya/webrtc"
	client.conn.Mode = core.ModeActiveProducer
	client.conn.Protocol = "mqtt"

	mqttClient := client.api.GetMqtt()
	if mqttClient == nil {
		err = errors.New("tuya: no mqtt client")
		client.Close(err)
		return nil, err
	}

	// Set up MQTT handlers
	mqttClient.handleAnswer = func(answer AnswerFrame) {
		// fmt.Printf("tuya: answer: %s\n", answer.Sdp)

		desc := pion.SessionDescription{
			Type: pion.SDPTypePranswer,
			SDP:  answer.Sdp,
		}

		if err = client.pc.SetRemoteDescription(desc); err != nil {
			client.Close(err)
			return
		}

		if err = client.conn.SetAnswer(answer.Sdp); err != nil {
			client.Close(err)
			return
		}

		if client.isHEVC {
			// Tuya seems to answers always with H264 and PCMU/8000 and PCMA/8000 codecs, replace with real codecs

			for _, media := range client.conn.Medias {
				if media.Kind == core.KindVideo {
					codecs := client.api.GetVideoCodecs()
					if codecs != nil {
						media.Codecs = codecs
					}
				}
			}

			for _, media := range client.conn.Medias {
				if media.Kind == core.KindAudio {
					codecs := client.api.GetAudioCodecs()
					if codecs != nil {
						media.Codecs = codecs
					}
				}
			}
		}
	}

	mqttClient.handleCandidate = func(candidate CandidateFrame) {
		// fmt.Printf("tuya: candidate: %s\n", candidate.Candidate)

		if candidate.Candidate != "" {
			client.conn.AddCandidate(candidate.Candidate)
			if err != nil {
				client.Close(err)
			}
		}
	}

	mqttClient.handleDisconnect = func() {
		// fmt.Println("tuya: disconnect")
		client.Close(errors.New("mqtt: disconnect"))
	}

	mqttClient.handleError = func(err error) {
		// fmt.Printf("tuya: error: %s\n", err.Error())
		client.Close(err)
	}

	// On HEVC, use DataChannel to receive video/audio
	if client.isHEVC {
		// Create a new DataChannel
		maxRetransmits := uint16(5)
		ordered := true
		client.dc, err = client.pc.CreateDataChannel("fmp4Stream", &pion.DataChannelInit{
			MaxRetransmits: &maxRetransmits,
			Ordered:        &ordered,
		})

		// Set up data channel handler
		client.dc.OnMessage(func(msg pion.DataChannelMessage) {
			if msg.IsString {
				if connected, err := client.probe(msg); err != nil {
					client.Close(err)
				} else if connected {
					client.connected.Done(nil)
				}
			} else {
				packet := &rtp.Packet{}
				if err := packet.Unmarshal(msg.Data); err != nil {
					// skip
					return
				}

				if handler, ok := client.handlers[packet.SSRC]; ok {
					handler(packet)
				}
			}
		})

		client.dc.OnError(func(err error) {
			// fmt.Printf("tuya: datachannel error: %s\n", err.Error())
			client.Close(err)
		})

		client.dc.OnClose(func() {
			// fmt.Println("tuya: datachannel closed")
			client.Close(errors.New("datachannel: closed"))
		})

		client.dc.OnOpen(func() {
			// fmt.Println("tuya: datachannel opened")

			codecRequest, _ := json.Marshal(DataChannelMessage{
				Type: "codec",
				Msg:  "",
			})

			if err := client.sendMessageToDataChannel(codecRequest); err != nil {
				client.Close(fmt.Errorf("failed to send codec request: %w", err))
			}
		})
	}

	// Set up pc handler
	client.conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()
			if err := mqttClient.SendCandidate("a=" + msg.ToJSON().Candidate); err != nil {
				client.Close(err)
			}

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateNew:
				break
			case pion.PeerConnectionStateConnecting:
				break
			case pion.PeerConnectionStateConnected:
				// On HEVC, wait for DataChannel to be opened and camera to send codec info
				if !client.isHEVC {
					if streamResolution == "hd" {
						_ = mqttClient.SendResolution(0)
					}
					client.connected.Done(nil)
				}
			default:
				client.Close(errors.New("webrtc: " + msg.String()))
			}
		}
	})

	// Audio first, otherwise tuya will send corrupt sdp
	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// Create offer
	offer, err := client.conn.CreateOffer(medias)
	if err != nil {
		client.Close(err)
		return nil, err
	}

	// horter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
	// https://github.com/tuya/webrtc-demo-go/blob/04575054f18ccccb6bc9d82939dd46d449544e20/static/js/main.js#L224
	re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
	offer = re.ReplaceAllString(offer, "")

	// Send offer
	if err := mqttClient.SendOffer(offer, streamResolution, client.streamType, client.isHEVC); err != nil {
		err = fmt.Errorf("tuya: %w", err)
		client.Close(err)
		return nil, err
	}

	sendOffer.Done(nil)

	// Wait for connection
	if err = client.connected.Wait(); err != nil {
		err = fmt.Errorf("tuya: %w", err)
		client.Close(err)
		return nil, err
	}

	return client, nil
}

func (c *Client) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	// Manually handle backchannel, because repacking audio through go2rtc does not work

	localTrack := c.getSender()
	if localTrack == nil {
		return errors.New("webrtc: can't get track")
	}

	mqttClient := c.api.GetMqtt()
	if mqttClient != nil {
		_ = mqttClient.SendSpeaker(1)
	}

	payloadType := codec.PayloadType

	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.conn.Send += packet.MarshalSize()
		//important to send with remote PayloadType
		_ = localTrack.WriteRTP(payloadType, packet)
	}

	sender.HandleRTP(track)
	c.conn.Senders = append(c.conn.Senders, sender)

	return nil
}

func (c *Client) Start() error {
	if len(c.conn.Receivers) == 0 {
		return errors.New("tuya: no receivers")
	}

	var video, audio *core.Receiver
	for _, receiver := range c.conn.Receivers {
		if receiver.Codec.IsVideo() {
			video = receiver
		} else if receiver.Codec.IsAudio() {
			audio = receiver
		}
	}

	c.handlers[c.videoSSRC] = func(packet *rtp.Packet) {
		if video != nil {
			video.WriteRTP(packet)
		}
	}

	c.handlers[c.audioSSRC] = func(packet *rtp.Packet) {
		if audio != nil {
			audio.WriteRTP(packet)
		}
	}

	return c.conn.Start()
}

func (c *Client) Stop() error {
	if c.closed {
		return nil
	}

	c.closed = true

	for ssrc := range c.handlers {
		delete(c.handlers, ssrc)
	}

	if c.conn != nil {
		_ = c.conn.Stop()
	}

	if c.api != nil {
		c.api.Close()
	}

	return nil
}

func (c *Client) Close(err error) error {
	c.connected.Done(err)
	return c.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}

func (c *Client) probe(msg pion.DataChannelMessage) (bool, error) {
	// fmt.Printf("[tuya] Received string message: %s\n", string(msg.Data))

	var message DataChannelMessage
	if err := json.Unmarshal([]byte(msg.Data), &message); err != nil {
		return false, err
	}

	switch message.Type {
	case "codec":
		frameRequest, _ := json.Marshal(DataChannelMessage{
			Type: "start",
			Msg:  "frame",
		})

		err := c.sendMessageToDataChannel(frameRequest)
		if err != nil {
			return false, err
		}

	case "recv":
		var recvMessage RecvMessage
		if err := json.Unmarshal([]byte(message.Msg), &recvMessage); err != nil {
			return false, err
		}

		c.videoSSRC = recvMessage.Video.SSRC
		c.audioSSRC = recvMessage.Audio.SSRC

		completeMsg, _ := json.Marshal(DataChannelMessage{
			Type: "complete",
			Msg:  "",
		})

		err := c.sendMessageToDataChannel(completeMsg)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (c *Client) sendMessageToDataChannel(message []byte) error {
	if c.dc != nil {
		// fmt.Printf("[tuya] sending message to data channel: %s\n", message)
		return c.dc.Send(message)
	}

	return nil
}

func (c *Client) getSender() *webrtc.Track {
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Kind() == pion.RTPCodecTypeAudio {
			if tr.Direction() == pion.RTPTransceiverDirectionSendonly || tr.Direction() == pion.RTPTransceiverDirectionSendrecv {
				if s := tr.Sender(); s != nil {
					if t := s.Track().(*webrtc.Track); t != nil {
						return t
					}
				}
			}
		}
	}
	return nil
}
