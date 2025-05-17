package tuya

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api       *TuyaClient
	conn      *webrtc.Conn
	pc        *pion.PeerConnection
	dc        *pion.DataChannel
	videoSSRC uint32
	audioSSRC uint32
	isHEVC    bool
	connected core.Waiter
	closed    bool
	handlers  map[uint32]func(*rtp.Packet)
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

const (
	DefaultCnURL        = "openapi.tuyacn.com"
	DefaultWestUsURL    = "openapi.tuyaus.com"
	DefaultEastUsURL    = "openapi-ueaz.tuyaus.com"
	DefaultCentralEuURL = "openapi.tuyaeu.com"
	DefaultWestEuURL    = "openapi-weaz.tuyaeu.com"
	DefaultInURL        = "openapi.tuyain.com"
)

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	deviceID := query.Get("device_id")
	uid := query.Get("uid")
	clientId := query.Get("client_id")
	clientSecret := query.Get("client_secret")
	streamRole := query.Get("role")
	streamMode := query.Get("mode")

	if streamRole == "" || (streamRole != "main" && streamRole != "sub") {
		streamRole = "main"
	}

	useRTSP := streamMode == "rtsp"
	useHLS := streamMode == "hls"
	useWebRTC := streamMode == "webrtc" || streamMode == ""

	// check if host is correct
	switch u.Hostname() {
	case DefaultCnURL:
	case DefaultWestUsURL:
	case DefaultEastUsURL:
	case DefaultCentralEuURL:
	case DefaultWestEuURL:
	case DefaultInURL:
	default:
		return nil, fmt.Errorf("tuya: wrong host %s", u.Hostname())
	}

	if deviceID == "" || clientId == "" || clientSecret == "" {
		return nil, errors.New("tuya: no device_id, client_id or client_secret")
	}

	if useWebRTC && uid == "" {
		return nil, errors.New("tuya: no uid")
	}

	if !useRTSP && !useHLS && !useWebRTC {
		return nil, errors.New("tuya: wrong stream type")
	}

	// Initialize Tuya API client
	tuyaAPI, err := NewTuyaClient(u.Hostname(), deviceID, uid, clientId, clientSecret, streamMode, streamRole)
	if err != nil {
		return nil, fmt.Errorf("tuya: %w", err)
	}

	client := &Client{
		api:      tuyaAPI,
		handlers: make(map[uint32]func(*rtp.Packet)),
	}

	if useRTSP {
		if client.api.rtspURL == "" {
			return nil, errors.New("tuya: no rtsp url")
		}
		return streams.GetProducer(client.api.rtspURL)
	} else if useHLS {
		if client.api.hlsURL == "" {
			return nil, errors.New("tuya: no hls url")
		}
		return streams.GetProducer(client.api.hlsURL)
	} else {
		client.isHEVC = client.api.isHEVC(client.api.getStreamType(streamRole))

		// Create a new PeerConnection
		conf := pion.Configuration{
			ICEServers:         client.api.iceServers,
			ICETransportPolicy: pion.ICETransportPolicyAll,
			BundlePolicy:       pion.BundlePolicyMaxBundle,
		}

		api, err := webrtc.NewAPI()
		if err != nil {
			client.api.Close()
			return nil, err
		}

		client.pc, err = api.NewPeerConnection(conf)
		if err != nil {
			client.api.Close()
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
		client.conn.URL = rawURL

		// Set up MQTT handlers
		client.api.mqtt.handleAnswer = func(answer AnswerFrame) {
			// fmt.Printf("tuya: answer: %s\n", answer.Sdp)

			desc := pion.SessionDescription{
				Type: pion.SDPTypePranswer,
				SDP:  answer.Sdp,
			}

			if err = client.pc.SetRemoteDescription(desc); err != nil {
				client.connected.Done(err)
				return
			}

			if err = client.conn.SetAnswer(answer.Sdp); err != nil {
				client.Stop()
				return
			}

			if client.isHEVC {
				// Tuya answers always with H264 codec, replace with HEVC
				for _, media := range client.conn.Medias {
					if media.Kind == core.KindVideo {
						for _, codec := range media.Codecs {
							if codec.Name == core.CodecH264 {
								codec.Name = core.CodecH265
							}
						}
					}
				}
			}
		}

		client.api.mqtt.handleCandidate = func(candidate CandidateFrame) {
			// fmt.Printf("tuya: candidate: %s\n", candidate.Candidate)

			if candidate.Candidate != "" {
				client.conn.AddCandidate(candidate.Candidate)
				if err != nil {
					client.Stop()
				}
			}
		}

		client.api.mqtt.handleDisconnect = func() {
			client.Stop()
		}

		client.api.mqtt.handleError = func(err error) {
			// fmt.Printf("tuya: error: %s\n", err.Error())
			client.Stop()
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
					client.probe(msg)
				} else {
					packet := &rtp.Packet{}
					if err := packet.Unmarshal(msg.Data); err != nil {
						return
					}

					if handler, ok := client.handlers[packet.SSRC]; ok {
						handler(packet)
					}
				}
			})

			client.dc.OnError(func(err error) {
				// fmt.Printf("tuya: datachannel error: %s\n", err.Error())
				client.connected.Done(err)
			})

			client.dc.OnClose(func() {
				// fmt.Println("tuya: datachannel closed")
				client.connected.Done(errors.New("datachannel: closed"))
			})

			client.dc.OnOpen(func() {
				// fmt.Println("tuya: datachannel opened")

				codecRequest, _ := json.Marshal(DataChannelMessage{
					Type: "codec",
					Msg:  "",
				})

				if err := client.sendMessageToDataChannel(codecRequest); err != nil {
					client.connected.Done(fmt.Errorf("failed to send codec request: %w", err))
				}
			})
		}

		// Set up pc handler
		client.conn.Listen(func(msg any) {
			switch msg := msg.(type) {
			case *pion.ICECandidate:
				_ = sendOffer.Wait()
				client.api.sendCandidate("a=" + msg.ToJSON().Candidate)

			case pion.PeerConnectionState:
				switch msg {
				case pion.PeerConnectionStateNew:
					break
				case pion.PeerConnectionStateConnecting:
					break
				case pion.PeerConnectionStateConnected:
					// On HEVC, wait for DataChannel to be opened and camera to send codec info
					if !client.isHEVC {
						client.connected.Done(nil)
					}
				default:
					client.Stop()
					client.connected.Done(errors.New("webrtc: " + msg.String()))
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
			client.api.Close()
			return nil, err
		}

		// horter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
		// https://github.com/tuya/webrtc-demo-go/blob/04575054f18ccccb6bc9d82939dd46d449544e20/static/js/main.js#L224
		re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
		offer = re.ReplaceAllString(offer, "")

		// Send offer
		client.api.sendOffer(offer, streamRole)
		sendOffer.Done(nil)

		// Wait for connection
		if err = client.connected.Wait(); err != nil {
			return nil, fmt.Errorf("tuya: %w", err)
		}

		return client, nil
	}
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

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}

func (c *Client) probe(msg pion.DataChannelMessage) {
	// fmt.Printf("[tuya] Received string message: %s\n", string(msg.Data))

	var message DataChannelMessage
	if err := json.Unmarshal([]byte(msg.Data), &message); err != nil {
		c.connected.Done(fmt.Errorf("failed to parse datachannel message: %w", err))
	}

	switch message.Type {
	case "codec":
		// fmt.Printf("[tuya] Codec info from camera: %s\n", message.Msg)

		frameRequest, _ := json.Marshal(DataChannelMessage{
			Type: "start",
			Msg:  "frame",
		})

		err := c.sendMessageToDataChannel(frameRequest)
		if err != nil {
			c.connected.Done(fmt.Errorf("failed to send frame request: %w", err))
		}

	case "recv":
		var recvMessage RecvMessage
		if err := json.Unmarshal([]byte(message.Msg), &recvMessage); err != nil {
			c.connected.Done(fmt.Errorf("failed to parse recv message: %w", err))
			return
		}

		c.videoSSRC = recvMessage.Video.SSRC
		c.audioSSRC = recvMessage.Audio.SSRC

		completeMsg, _ := json.Marshal(DataChannelMessage{
			Type: "complete",
			Msg:  "",
		})

		err := c.sendMessageToDataChannel(completeMsg)
		if err != nil {
			c.connected.Done(fmt.Errorf("failed to send complete message: %w", err))
		}

		c.connected.Done(nil)
	}
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
			if tr.Kind() == pion.RTPCodecType(pion.RTPTransceiverDirectionSendonly) || tr.Kind() == pion.RTPCodecType(pion.RTPTransceiverDirectionSendrecv) {
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
