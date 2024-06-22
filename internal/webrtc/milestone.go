package webrtc

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

// This package handles the Milestone WebRTC session lifecycle, including authentication,
// session creation, and session update with an SDP answer. It is designed to be used with
// a specific URL format that encodes session parameters. For example:
// webrtc:https://milestone-host/api#format=milestone#username=User#password=TestPassword#cameraId=a539f254-af05-4d67-a1bb-cd9b3c74d122
//
// https://github.com/milestonesys/mipsdk-samples-protocol/tree/main/WebRTC_JavaScript

type milestoneAPI struct {
	url       string
	query     url.Values
	token     string
	sessionID string
}

func (m *milestoneAPI) GetToken() error {
	data := url.Values{
		"client_id":  {"GrantValidatorClient"},
		"grant_type": {"password"},
		"username":   m.query["username"],
		"password":   m.query["password"],
	}

	req, err := http.NewRequest("POST", m.url+"/IDP/connect/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// support httpx protocol
	res, err := tcp.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New("milesone: authentication failed: " + res.Status)
	}

	var payload map[string]interface{}
	if err = json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return err
	}

	token, ok := payload["access_token"].(string)
	if !ok {
		return errors.New("milesone: token not found in the response")
	}

	m.token = token

	return nil
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func (m *milestoneAPI) GetOffer() (string, error) {
	request := struct {
		CameraId         string `json:"cameraId"`
		StreamId         string `json:"streamId,omitempty"`
		PlaybackTimeNode struct {
			PlaybackTime string  `json:"playbackTime,omitempty"`
			SkipGaps     bool    `json:"skipGaps,omitempty"`
			Speed        float64 `json:"speed,omitempty"`
		} `json:"playbackTimeNode,omitempty"`
		//ICEServers []string `json:"iceServers,omitempty"`
		//Resolution string   `json:"resolution,omitempty"`
	}{
		CameraId: m.query.Get("cameraId"),
		StreamId: m.query.Get("streamId"),
	}
	request.PlaybackTimeNode.PlaybackTime = m.query.Get("playbackTime")
	request.PlaybackTimeNode.SkipGaps = m.query.Has("skipGaps")
	request.PlaybackTimeNode.Speed = parseFloat(m.query.Get("speed"))

	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", m.url+"/REST/v1/WebRTC/Session", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := tcp.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", errors.New("milesone: create session: " + res.Status)
	}

	var response struct {
		SessionId string `json:"sessionId"`
		OfferSDP  string `json:"offerSDP"`
	}
	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", err
	}

	var offer pion.SessionDescription
	if err = json.Unmarshal([]byte(response.OfferSDP), &offer); err != nil {
		return "", err
	}

	m.sessionID = response.SessionId

	return offer.SDP, nil
}

func (m *milestoneAPI) SetAnswer(sdp string) error {
	answer := pion.SessionDescription{
		Type: pion.SDPTypeAnswer,
		SDP:  sdp,
	}
	data, err := json.Marshal(answer)
	if err != nil {
		return err
	}

	request := struct {
		AnswerSDP string `json:"answerSDP"`
	}{
		AnswerSDP: string(data),
	}
	if data, err = json.Marshal(request); err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", m.url+"/REST/v1/WebRTC/Session/"+m.sessionID, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := tcp.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New("milesone: patch session: " + res.Status)
	}

	return nil
}

func milestoneClient(rawURL string, query url.Values) (core.Producer, error) {
	mc := &milestoneAPI{url: rawURL, query: query}
	if err := mc.GetToken(); err != nil {
		return nil, err
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	conf := pion.Configuration{}
	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/milestone"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "http"
	prod.URL = rawURL

	offer, err := mc.GetOffer()
	if err != nil {
		return nil, err
	}

	if err = prod.SetOffer(offer); err != nil {
		return nil, err
	}

	answer, err := prod.GetAnswer()
	if err != nil {
		return nil, err
	}

	if err = mc.SetAnswer(answer); err != nil {
		return nil, err
	}

	return prod, nil
}
