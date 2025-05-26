package tuya

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tuya"
)

func Init() {
	streams.HandleFunc("tuya", func(source string) (core.Producer, error) {
		return tuya.Dial(source)
	})

	api.HandleFunc("api/tuya", apiTuya)
}

func apiTuya(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	region := query.Get("region")
	email := query.Get("email")
	password := query.Get("password")

	if email == "" || password == "" || region == "" {
		http.Error(w, "email, password and region are required", http.StatusBadRequest)
		return
	}

	var tuyaRegion *tuya.Region
	for _, r := range tuya.AvailableRegions {
		if r.Host == region {
			tuyaRegion = &r
			break
		}
	}

	if tuyaRegion == nil {
		http.Error(w, fmt.Sprintf("invalid region: %s", region), http.StatusBadRequest)
		return
	}

	httpClient := tuya.CreateHTTPClientWithSession()

	_, err := login(httpClient, tuyaRegion.Host, email, password, tuyaRegion.Continent)
	if err != nil {
		http.Error(w, fmt.Sprintf("login failed: %v", err), http.StatusInternalServerError)
		return
	}

	tuyaAPI, err := tuya.NewTuyaSmartApiClient(
		httpClient,
		tuyaRegion.Host,
		email,
		password,
		"",
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var devices []tuya.Device

	homes, _ := tuyaAPI.GetHomeList()
	if homes != nil && len(homes.Result) > 0 {
		for _, home := range homes.Result {
			roomList, err := tuyaAPI.GetRoomList(strconv.Itoa(home.Gid))
			if err != nil {
				continue
			}

			for _, room := range roomList.Result {
				for _, device := range room.DeviceList {
					if (device.Category == "sp" || device.Category == "dghsxj") && !containsDevice(devices, device.DeviceId) {
						devices = append(devices, device)
					}
				}
			}
		}
	}

	sharedHomes, _ := tuyaAPI.GetSharedHomeList()
	if sharedHomes != nil && len(sharedHomes.Result.SecurityWebCShareInfoList) > 0 {
		for _, sharedHome := range sharedHomes.Result.SecurityWebCShareInfoList {
			for _, device := range sharedHome.DeviceInfoList {
				if (device.Category == "sp" || device.Category == "dghsxj") && !containsDevice(devices, device.DeviceId) {
					devices = append(devices, device)
				}
			}
		}
	}

	if len(devices) == 0 {
		http.Error(w, "no cameras found", http.StatusNotFound)
		return
	}

	var items []*api.Source
	for _, device := range devices {
		cleanQuery := url.Values{}
		cleanQuery.Set("device_id", device.DeviceId)
		cleanQuery.Set("email", email)
		cleanQuery.Set("password", password)
		url := fmt.Sprintf("tuya://%s?%s", tuyaRegion.Host, cleanQuery.Encode())

		items = append(items, &api.Source{
			Name: device.DeviceName,
			URL:  url,
		})
	}

	api.ResponseSources(w, items)
}

func login(client *http.Client, serverHost, email, password, countryCode string) (*tuya.LoginResult, error) {
	tokenResp, err := getLoginToken(client, serverHost, email, countryCode)
	if err != nil {
		return nil, err
	}

	encryptedPassword, err := tuya.EncryptPassword(password, tokenResp.Result.PbKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %v", err)
	}

	var loginResp *tuya.PasswordLoginResponse
	var url string

	loginReq := tuya.PasswordLoginRequest{
		CountryCode: countryCode,
		Passwd:      encryptedPassword,
		Token:       tokenResp.Result.Token,
		IfEncrypt:   1,
		Options:     `{"group":1}`,
	}

	if tuya.IsEmailAddress(email) {
		url = fmt.Sprintf("https://%s/api/private/email/login", serverHost)
		loginReq.Email = email
	} else {
		url = fmt.Sprintf("https://%s/api/private/phone/login", serverHost)
		loginReq.Mobile = email
	}

	loginResp, err = performLogin(client, url, loginReq, serverHost)

	if err != nil {
		return nil, err
	}

	if !loginResp.Success {
		return nil, errors.New(loginResp.ErrorMsg)
	}

	return &loginResp.Result, nil
}

func getLoginToken(client *http.Client, serverHost, username, countryCode string) (*tuya.LoginTokenResponse, error) {
	url := fmt.Sprintf("https://%s/api/login/token", serverHost)

	tokenReq := tuya.LoginTokenRequest{
		CountryCode: countryCode,
		Username:    username,
		IsUid:       false,
	}

	jsonData, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", serverHost))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", serverHost))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp tuya.LoginTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if !tokenResp.Success {
		return nil, err
	}

	return &tokenResp, nil
}

func performLogin(client *http.Client, url string, loginReq tuya.PasswordLoginRequest, serverHost string) (*tuya.PasswordLoginResponse, error) {
	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", serverHost))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", serverHost))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var loginResp tuya.PasswordLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, err
	}

	return &loginResp, nil
}

func containsDevice(devices []tuya.Device, deviceID string) bool {
	for _, device := range devices {
		if device.DeviceId == deviceID {
			return true
		}
	}
	return false
}
