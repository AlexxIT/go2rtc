package tuya

import (
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
	api  *TuyaClient
	conn *webrtc.Conn
	done chan struct{}
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
	// Parse URL and validate basic params
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	deviceID := query.Get("device_id")
	uid := query.Get("uid")
	clientId := query.Get("client_id")
	clientSecret := query.Get("client_secret")
	streamType := query.Get("type")
	streamMode := query.Get("mode")
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
	tuyaAPI, err := NewTuyaClient(u.Hostname(), deviceID, uid, clientId, clientSecret, streamType)
	if err != nil {
		return nil, err
	}

	client := &Client{
		api:  tuyaAPI,
		done: make(chan struct{}),
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
		conf := pion.Configuration{
			ICEServers:         client.api.iceServers,
			ICETransportPolicy: pion.ICETransportPolicyAll,
			BundlePolicy:       pion.BundlePolicyBalanced,
		}

		api, err := webrtc.NewAPI()
		if err != nil {
			client.api.Close()
			return nil, err
		}

		pc, err := api.NewPeerConnection(conf)
		if err != nil {
			client.api.Close()
			return nil, err
		}

		// protect from sending ICE candidate before Offer
		var sendOffer core.Waiter

		// protect from blocking on errors
		defer sendOffer.Done(nil)

		// waiter will wait PC error or WS error or nil (connection OK)
		var connState core.Waiter

		client.conn = webrtc.NewConn(pc)
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

			if err = pc.SetRemoteDescription(desc); err != nil {
				client.Stop()
				return
			}

			if err = client.conn.SetAnswer(answer.Sdp); err != nil {
				client.Stop()
				return
			}

			client.conn.SDP = answer.Sdp
		}

		client.api.mqtt.handleCandidate = func(candidate CandidateFrame) {
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
					connState.Done(nil)
				default:
					connState.Done(errors.New("webrtc: " + msg.String()))
				}
			}
		})

		// Create offer
		offer, err := client.conn.CreateOffer(client.api.medias)
		if err != nil {
			client.api.Close()
			return nil, err
		}

		// horter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
		// https://github.com/tuya/webrtc-demo-go/blob/04575054f18ccccb6bc9d82939dd46d449544e20/static/js/main.js#L224
		re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
		offer = re.ReplaceAllString(offer, "")

		// Send offer
		client.api.sendOffer(offer, tuyaAPI.getStreamType(streamType))
		sendOffer.Done(nil)

		if err = connState.Wait(); err != nil {
			return nil, err
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
	// RepackG711 will not work, so add default logic without repacking

	payloadType := codec.PayloadType

	localTrack := c.conn.GetSenderTrack(media.ID)
	if localTrack == nil {
		return errors.New("webrtc: can't get track")
	}

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
	return c.conn.Start()
}

func (c *Client) Stop() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
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
