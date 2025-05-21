package tuya

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tuya"
)

var users = make(map[string]tuya.LoginResponse)

func Init() {
	streams.HandleFunc("tuya", func(source string) (core.Producer, error) {
		return tuya.Dial(source)
	})

	api.HandleFunc("api/tuya", apiTuya)
}

func apiTuya(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	userCode := query.Get("user_code")
	token := query.Get("token")

	if userCode == "" {
		http.Error(w, "user_code is required", http.StatusBadRequest)
		return
	}

	var auth *tuya.LoginResponse
	if loginResponse, ok := users[userCode]; ok {
		expireTime := loginResponse.Timestamp + loginResponse.Result.ExpireTime

		if expireTime > time.Now().Unix() {
			auth = &loginResponse
		} else {
			delete(users, userCode)
			token = ""
		}
	}

	if auth == nil && token == "" {
		qrCode, err := getQRCode(userCode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// response qrCode
		json.NewEncoder(w).Encode(map[string]interface{}{
			"qrCode": qrCode,
		})

		return
	}

	if auth == nil && token != "" {
		authResponse, err := login(userCode, token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		auth = authResponse
	}

	if auth == nil {
		http.Error(w, "failed to get auth", http.StatusInternalServerError)
		return
	}

	tokenInfo := tuya.TokenInfo{
		AccessToken:  auth.Result.AccessToken,
		ExpireTime:   auth.Timestamp + auth.Result.ExpireTime,
		RefreshToken: auth.Result.RefreshToken,
	}

	tokenInfoBase64, err := tuya.ToBase64(&tokenInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tuyaAPI, err := tuya.NewTuyaOpenApiClient(
		strings.Replace(auth.Result.Endpoint, "https://", "", 1),
		auth.Result.UID,
		"",
		auth.Result.TerminalID,
		tokenInfo,
		"",
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	devices, err := tuyaAPI.GetAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*api.Source
	for _, device := range devices {
		cleanQuery := url.Values{}
		cleanQuery.Set("uid", auth.Result.UID)
		cleanQuery.Set("token", tokenInfoBase64)
		cleanQuery.Set("terminal_id", auth.Result.TerminalID)
		cleanQuery.Set("device_id", device.ID)

		endpoint := strings.Replace(auth.Result.Endpoint, "https://", "tuya://", 1)
		url := fmt.Sprintf("%s?%s", endpoint, cleanQuery.Encode())

		items = append(items, &api.Source{
			Name: device.Name,
			URL:  url,
		})
	}

	api.ResponseSources(w, items)
}

func login(userCode string, qrCode string) (*tuya.LoginResponse, error) {
	url := fmt.Sprintf("https://%s/v1.0/m/life/home-assistant/qrcode/tokens/%s?clientid=%s&usercode=%s", tuya.TUYA_HOST, qrCode, tuya.TUYA_CLIENT_ID, userCode)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	res, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get QR code: %s", string(res))
	}

	var loginResponse tuya.LoginResponse
	err = json.Unmarshal(res, &loginResponse)
	if err != nil {
		return nil, err
	}

	if !loginResponse.Success {
		return nil, fmt.Errorf("failed to login: %s", loginResponse.Msg)
	}

	users[userCode] = loginResponse

	return &loginResponse, nil
}

func getQRCode(userCode string) (string, error) {
	url := fmt.Sprintf("https://%s/v1.0/m/life/home-assistant/qrcode/tokens?clientid=%s&schema=%s&usercode=%s", tuya.TUYA_HOST, tuya.TUYA_CLIENT_ID, tuya.TUYA_SCHEMA, userCode)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "text/plain")

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	res, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	if response.StatusCode != http.StatusOK {
		return "", err
	}

	var qrResponse tuya.QRResponse
	err = json.Unmarshal(res, &qrResponse)
	if err != nil {
		return "", err
	}

	if !qrResponse.Success {
		return "", fmt.Errorf("failed to get QR code: %s", qrResponse.Msg)
	}

	return qrResponse.Result.Code, nil
}
