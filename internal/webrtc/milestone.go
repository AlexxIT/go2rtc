package webrtc

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

// This package handles the Milestone WebRTC session lifecycle, including authentication,
// session creation, and session update with an SDP answer. It is designed to be used with
// a specific URL format that encodes session parameters. For example:
// webrtc:https://milestone-host/api#format=milestone#username=User#password=TestPassword#cameraId=a539f254-af05-4d67-a1bb-cd9b3c74d122
// see: https://github.com/milestonesys/mipsdk-samples-protocol/tree/main/WebRTC_JavaScript

// MilestoneClient manages the configurations of the server and client
type MilestoneClient struct {
	ApiGatewayUrl  string
	Username       string
	Password       string
	ClientID       string
	Token          string
	GrantType      string
	PeerConnection *pion.PeerConnection
}

// WebRTCSessionDetails structures the session details for the WebRTC connection.
type WebRTCSessionDetails struct {
	CameraId         string               `json:"cameraId"`
	StreamId         *string              `json:"streamId,omitempty"`
	PlaybackTimeNode *PlaybackTimeDetails `json:"playbackTimeNode,omitempty"`
	ICEServers       []string             `json:"iceServers"`
	Resolution       string               `json:"resolution"`
}

// PlaybackTimeDetails holds optional playback parameters
type PlaybackTimeDetails struct {
	PlaybackTime string   `json:"playbackTime"`
	SkipGaps     *bool    `json:"skipGaps,omitempty"`
	Speed        *float64 `json:"speed,omitempty"`
}

func setupMilestoneClient(rawURL string, query url.Values) *MilestoneClient {
	return &MilestoneClient{
		ApiGatewayUrl: rawURL,
		Username:      query.Get("username"),
		Password:      query.Get("password"),
		ClientID:      "GrantValidatorClient",
		GrantType:     "password",
	}
}

func parseSessionDetails(query url.Values) WebRTCSessionDetails {
	details := WebRTCSessionDetails{
		CameraId:   query.Get("cameraId"),
		Resolution: "notInUse",
		ICEServers: []string{},
	}

	if streamId := query.Get("streamId"); streamId != "" {
		details.StreamId = &streamId
	}

	// Check for playback related details and construct PlaybackTimeNode if necessary
	var playbackTimeNode PlaybackTimeDetails
	hasPlaybackDetails := false

	if playbackTime := query.Get("playbackTime"); playbackTime != "" {
		playbackTimeNode.PlaybackTime = playbackTime
		hasPlaybackDetails = true
	}

	if skipGaps := query.Get("skipGaps"); skipGaps != "" {
		skipGapsBool, err := strconv.ParseBool(skipGaps)
		if err == nil {
			playbackTimeNode.SkipGaps = &skipGapsBool
			hasPlaybackDetails = true
		}
	}

	if speed := query.Get("speed"); speed != "" {
		speedFloat, err := strconv.ParseFloat(speed, 64)
		if err == nil {
			playbackTimeNode.Speed = &speedFloat
			hasPlaybackDetails = true
		}
	}

	if hasPlaybackDetails {
		details.PlaybackTimeNode = &playbackTimeNode
	}

	return details
}

func createWebRTCSession(mc *MilestoneClient, details WebRTCSessionDetails) (*http.Response, error) {
	body, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}

	url := mc.ApiGatewayUrl + "/REST/v1/WebRTC/Session"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mc.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client.Do(req)
}

func updateWebRTCSession(mc *MilestoneClient, sessionID string, answer pion.SessionDescription) (*http.Response, error) {
	sdpJSON, err := json.Marshal(answer)
	if err != nil {
		return nil, err
	}

	payload := fmt.Sprintf(`{"answerSDP":%s}`, strconv.Quote(string(sdpJSON)))
	url := fmt.Sprintf("%s/REST/v1/WebRTC/Session/%s", mc.ApiGatewayUrl, sessionID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mc.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client.Do(req)
}

func (mc *MilestoneClient) Authenticate() error {
	formData := url.Values{
		"grant_type": {mc.GrantType},
		"username":   {mc.Username},
		"password":   {mc.Password},
		"client_id":  {mc.ClientID},
	}

	resp, err := http.PostForm(mc.ApiGatewayUrl+"/IDP/connect/token", formData)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed: status code %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	token, ok := result["access_token"].(string)
	if !ok {
		return errors.New("token not found in the response")
	}
	mc.Token = token

	return nil
}

func milestoneClient(rawURL string, query url.Values, desc string) (core.Producer, error) {
	mc := setupMilestoneClient(rawURL, query)

	if err := mc.Authenticate(); err != nil {
		return nil, err
	}

	details := parseSessionDetails(query)

	config := pion.Configuration{
		ICEServers: []pion.ICEServer{
			{
				URLs: details.ICEServers,
			},
		},
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	mc.PeerConnection, err = api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	var sendOffer core.Waiter
	defer sendOffer.Done(nil)

	prod := webrtc.NewConn(mc.PeerConnection)
	prod.Desc = "WebRTC/OpenIPC"
	prod.Mode = core.ModeActiveProducer

	resp, err := createWebRTCSession(mc, details)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var session struct {
		SessionId string `json:"sessionId"`
		OfferSDP  string `json:"offerSDP"`
	}

	if err := json.Unmarshal(responseBody, &session); err != nil {
		return nil, fmt.Errorf("error parsing session response: %v", err)
	}

	var offer pion.SessionDescription
	if err := json.Unmarshal([]byte(session.OfferSDP), &offer); err != nil {
		return nil, fmt.Errorf("failed to parse offer SDP: %v", err)
	}

	if err = prod.SetOffer(string(offer.SDP)); err != nil {
		return nil, err
	}

	if err := mc.PeerConnection.SetRemoteDescription(offer); err != nil {
		return nil, fmt.Errorf("failed to set remote description: %v", err)
	}

	answer, err := mc.PeerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create answer: %v", err)
	}

	if err := mc.PeerConnection.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %v", err)
	}

	resp, err = updateWebRTCSession(mc, session.SessionId, answer)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with non-OK status: %d", resp.StatusCode)
	}

	return prod, nil
}
