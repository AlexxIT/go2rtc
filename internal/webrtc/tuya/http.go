package tuya

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

func (t *tuyaSession) makeHttpSign(ts int64) string {
	// If httpAccessToken is "" then this is un-authed request, so no need to do 'if' here
	data := fmt.Sprintf("%s%s%s%d", t.config.ClientID, t.httpAccessToken, t.config.Secret, ts)
	val := md5.Sum([]byte(data))
	res := fmt.Sprintf("%X", val)
	return res
}

func (t *tuyaSession) httpRequest(method string, path string, body io.Reader) (res []byte, err error) {
	client := &http.Client{
		Timeout: time.Second * 5,
	}

	url := fmt.Sprintf("%s%s", t.config.OpenAPIURL, path)

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Printf("create http request fail: %s", err.Error())

		return
	}

	ts := time.Now().UnixNano() / 1000000
	sign := t.makeHttpSign(ts)

	// TODO: do we need all this headers?

	request.Header.Set("Accept", "*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Access-Control-Allow-Origin", "*")
	request.Header.Set("Access-Control-Allow-Methods", "*")
	request.Header.Set("Access-Control-Allow-Headers", "*")
	request.Header.Set("mode", "no-cors")
	request.Header.Set("client_id", t.config.ClientID)
	request.Header.Set("access_token", t.httpAccessToken)
	request.Header.Set("sign", sign)
	request.Header.Set("t", strconv.FormatInt(ts, 10))

	response, err := client.Do(request)
	if err != nil {
		log.Printf("http request fail: %s", err.Error())

		return
	}
	defer response.Body.Close()

	res, err = io.ReadAll(response.Body)
	if err != nil {
		log.Printf("read http response fail: %s", err.Error())

		return
	}

	return
}

func (t *tuyaSession) Authorize() (err error) {
	t.httpAccessToken = "" // Clear all access token if present

	body, err := t.httpRequest("GET", "/v1.0/token?grant_type=1", nil)
	if err != nil {
		log.Printf("sync OpenAPI ressponse to config fail: %s", err.Error())
		return
	}

	accessTokenValue := gjson.GetBytes(body, "result.access_token")
	if !accessTokenValue.Exists() {
		log.Printf("access_token not exits in body: %s", string(body))
		return errors.New("access_token not exist")
	}

	t.httpAccessToken = accessTokenValue.String()

	return
}

func (t *tuyaSession) GetMotoIDAndAuth() (motoID, auth, iceServers string, err error) {
	path := fmt.Sprintf("/v1.0/users/%s/devices/%s/webrtc-configs", t.config.UID, t.config.DeviceID)

	body, err := t.httpRequest("GET", path, nil)
	if err != nil {
		log.Printf("GET webrtc-configs fail: %s, body: %s", err.Error(), string((body)))

		return
	}

	motoIDValue := gjson.GetBytes(body, "result.moto_id")
	if !motoIDValue.Exists() {
		log.Printf("moto_id not exist in webrtc-configs, body: %s", string(body))

		return "", "", "", errors.New("moto_id not exist")
	}

	authValue := gjson.GetBytes(body, "result.auth")
	if !authValue.Exists() {
		log.Printf("auth not exist in webrtc-configs, body: %s", string(body))

		return "", "", "", errors.New("auth not exist")
	}

	iceServersValue := gjson.GetBytes(body, "result.p2p_config.ices")
	if !iceServersValue.Exists() {
		log.Printf("iceServers not exist in webrtc-configs, body: %s", string(body))

		return "", "", "", errors.New("p2p_config.ices not exist")
	}

	var tokens []Token
	err = json.Unmarshal([]byte(iceServersValue.String()), &tokens)
	if err != nil {
		log.Printf("unmarshal to tokens fail: %s", err.Error())
		return "", "", "", err
	}

	ices := make([]WebToken, 0)
	for _, token := range tokens {
		if strings.HasPrefix(token.Urls, "stun") {
			ices = append(ices, WebToken{
				Urls: token.Urls,
			})
		} else if strings.HasPrefix(token.Urls, "turn") {
			ices = append(ices, WebToken{
				Urls:       token.Urls,
				Username:   token.Username,
				Credential: token.Credential,
			})
		}
	}

	iceServersBytes, err := json.Marshal(&ices)
	if err != nil {
		log.Printf("marshal token to web tokens fail: %s", err.Error())
		return "", "", "", err
	}

	motoID = motoIDValue.String()
	auth = authValue.String()
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

	payload, err := json.Marshal(request)
	if err != nil {
		log.Printf("marshal OpenIoTHubConfig Request fail: %s", err.Error())
		return nil, err
	}

	body, err := t.httpRequest("POST", "/v2.0/open-iot-hub/access/config", bytes.NewReader(payload))

	if err != nil {
		log.Printf("get OpenIoTHub config from http fail: %s", err.Error())
		return
	}

	if !gjson.GetBytes(body, "success").Bool() {
		log.Printf("request OpenIoTHub Config fail, body: %s", string(body))
		return nil, errors.New("request hub config fail")
	}

	config = &OpenIoTHubConfig{}

	err = json.Unmarshal([]byte(gjson.GetBytes(body, "result").String()), config)
	if err != nil {
		log.Printf("unmarshal OpenIoTHub config to object fail: %s, body: %s", err.Error(), string(body))
		return
	}

	return
}
