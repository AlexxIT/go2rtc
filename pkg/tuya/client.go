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
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api  *TuyaClient
	prod core.Producer
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
			BundlePolicy:       pion.BundlePolicyMaxBundle,
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

		prod := webrtc.NewConn(pc)
		prod.FormatName = "tuya/webrtc"
		prod.Mode = core.ModeActiveProducer
		prod.Protocol = "mqtt"
		prod.URL = rawURL

		client.prod = prod

		// Set up MQTT handlers
		client.api.mqtt.handleAnswer = func(answer AnswerFrame) {
			desc := pion.SessionDescription{
				Type: pion.SDPTypePranswer,
				SDP:  answer.Sdp,
			}

			if err = pc.SetRemoteDescription(desc); err != nil {
				client.Stop()
				return
			}

			if err = prod.SetAnswer(answer.Sdp); err != nil {
				client.Stop()
				return
			}

			prod.SDP = answer.Sdp
		}

		client.api.mqtt.handleCandidate = func(candidate CandidateFrame) {
			if candidate.Candidate != "" {
				prod.AddCandidate(candidate.Candidate)
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

		prod.Listen(func(msg any) {
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
		offer, err := prod.CreateOffer(client.api.medias)
		if err != nil {
			client.api.Close()
			return nil, err
		}

		// horter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
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
	return c.prod.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.prod.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if prod, ok := c.prod.(*webrtc.Conn); ok {
		return prod.AddTrack(media, codec, track)
	}

	return nil
}

func (c *Client) Start() error {
	return c.prod.Start()
}

func (c *Client) Stop() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}

	if c.prod != nil {
		_ = c.prod.Stop()
	}

	if c.api != nil {
		c.api.Close()
	}

	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	if webrtcProd, ok := c.prod.(*webrtc.Conn); ok {
		return webrtcProd.MarshalJSON()
	}

	return json.Marshal(c.prod)
}
