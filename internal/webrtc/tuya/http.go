package tuya

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (t *tuyaSession) makeHttpSign(ts int64) string {
	// If httpAccessToken is "" then this is un-authed request, so no need to do 'if' here
	data := fmt.Sprintf("%s%s%s%d", t.config.ClientID, t.httpAccessToken, t.config.Secret, ts)
	val := md5.Sum([]byte(data))
	res := fmt.Sprintf("%X", val)
	return res
}

func (t *tuyaSession) httpRequest(method string, path string, payload interface{}, response interface{}) (err error) {

	client := &http.Client{
		Timeout: time.Second * 5,
	}

	url := fmt.Sprintf("%s%s", t.config.OpenAPIURL, path)

	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("httpRequest: marshal payload: %w", err)
		}
		body = bytes.NewReader(payloadBytes)
	}

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("httpRequest: create request: %w", err)
	}

	ts := time.Now().UnixNano() / 1000000
	sign := t.makeHttpSign(ts)

	request.Header.Set("Accept", "*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("client_id", t.config.ClientID)
	request.Header.Set("access_token", t.httpAccessToken)
	request.Header.Set("sign", sign)
	request.Header.Set("t", strconv.FormatInt(ts, 10))

	rawResponse, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("httpRequest: do request: %w", err)
	}
	defer rawResponse.Body.Close()

	if rawResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("httpRequest: HTTP status %d", rawResponse.StatusCode)
	}

	responseBody, err := io.ReadAll(rawResponse.Body)
	if err != nil {
		return fmt.Errorf("httpRequest: read response: %w", err)
	}

	baseResponse := BaseHttpResponse{}
	if err := json.Unmarshal(responseBody, &baseResponse); err != nil {
		return fmt.Errorf("httpRequest: unmarshal base response: %w", err)
	}

	if !baseResponse.Success {
		return fmt.Errorf("httpRequest: Tuya API error: %s", string(responseBody))
	}

	if err := json.Unmarshal(baseResponse.Result, response); err != nil {
		return fmt.Errorf("httpRequest: unmarshal result: %w", err)
	}

	return nil
}

func (t *tuyaSession) Authorize() (err error) {
	t.httpAccessToken = "" // Clear all access token if present

	resp := AuthorizeResponse{}
	err = t.httpRequest("GET", "/v1.0/token?grant_type=1", nil, &resp)
	if err != nil {
		return err
	}

	t.httpAccessToken = resp.AccessToken

	return nil
}

func (t *tuyaSession) GetMotoIDAndAuth() (motoID, auth, iceServers string, err error) {
	path := fmt.Sprintf("/v1.0/users/%s/devices/%s/webrtc-configs", t.config.UID, t.config.DeviceID)

	resp := GetWebrtcConfigsResponse{}

	err = t.httpRequest("GET", path, nil, &resp)

	if err != nil {
		return
	}

	iceServersBytes, err := json.Marshal(&resp.P2PConfig.Tokens)
	if err != nil {
		log.Error().Msgf("marshal token to web tokens fail: %s", err.Error())
		return "", "", "", err
	}

	auth = resp.Auth
	motoID = resp.MotoId
	iceServers = string(iceServersBytes)

	return
}

func (t *tuyaSession) GetHubConfig() (config *OpenIoTHubConfig, err error) {
	request := &OpenIoTHubConfigRequest{
		UID:      t.config.UID,
		UniqueID: uuid.New().String(),
		LinkType: "mqtt",
		Topics:   "ipc",
	}

	err = t.httpRequest("POST", "/v2.0/open-iot-hub/access/config", &request, &config)
	if err != nil {
		return nil, err
	}

	return
}
