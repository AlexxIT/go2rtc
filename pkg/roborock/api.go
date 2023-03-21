package roborock

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type UserInfo struct {
	Token string `json:"token"`
	IoT   struct {
		User   string `json:"u"`
		Pass   string `json:"s"`
		Hash   string `json:"h"`
		Domain string `json:"k"`
		URL    struct {
			API  string `json:"a"`
			MQTT string `json:"m"`
		} `json:"r"`
	} `json:"rriot"`
}

func GetBaseURL(username string) (string, error) {
	u := "https://euiot.roborock.com/api/v1/getUrlByEmail?email=" + url.QueryEscape(username)
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return "", err
	}

	client := http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)

	var v struct {
		Msg  string `json:"msg"`
		Code int    `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", err
	}

	if v.Code != 200 {
		return "", fmt.Errorf("%d: %s", v.Code, v.Msg)
	}

	return v.Data.URL, nil
}

func Login(baseURL, username, password string) (*UserInfo, error) {
	u := baseURL + "/api/v1/login?username=" + url.QueryEscape(username) +
		"&password=" + url.QueryEscape(password) + "&needtwostepauth=false"
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return nil, err
	}

	clientID := core.RandString(16, 64)
	clientID = base64.StdEncoding.EncodeToString([]byte(clientID))
	req.Header.Set("header_clientid", clientID)

	client := http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)

	var v struct {
		Msg  string   `json:"msg"`
		Code int      `json:"code"`
		Data UserInfo `json:"data"`
	}
	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return nil, err
	}

	if v.Code != 200 {
		return nil, fmt.Errorf("%d: %s", v.Code, v.Msg)
	}

	return &v.Data, nil
}

func GetHomeID(baseURL, token string) (int, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/v1/getHomeDetail", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", token)

	client := http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	var v struct {
		Msg  string `json:"msg"`
		Code int    `json:"code"`
		Data struct {
			HomeID int `json:"rrHomeId"`
		} `json:"data"`
	}
	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return 0, err
	}

	if v.Code != 200 {
		return 0, fmt.Errorf("%d: %s", v.Code, v.Msg)
	}

	return v.Data.HomeID, nil
}

type DeviceInfo struct {
	DID  string `json:"duid"`
	Name string `json:"name"`
	Key  string `json:"localKey"`
}

func GetDevices(ui *UserInfo, homeID int) ([]DeviceInfo, error) {
	nonce := core.RandString(6, 64)
	ts := time.Now().Unix()
	path := "/user/homes/" + strconv.Itoa(homeID)

	mac := fmt.Sprintf(
		"%s:%s:%s:%d:%x::", ui.IoT.User, ui.IoT.Pass, nonce, ts, md5.Sum([]byte(path)),
	)
	hash := hmac.New(sha256.New, []byte(ui.IoT.Hash))
	hash.Write([]byte(mac))
	mac = base64.StdEncoding.EncodeToString(hash.Sum(nil))

	auth := fmt.Sprintf(
		`Hawk id="%s", s="%s", ts="%d", nonce="%s", mac="%s"`,
		ui.IoT.User, ui.IoT.Pass, ts, nonce, mac,
	)

	req, err := http.NewRequest("GET", ui.IoT.URL.API+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)

	client := http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	var v struct {
		Result struct {
			Devices []DeviceInfo `json:"devices"`
		} `json:"result"`
	}
	if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
		return nil, err
	}

	return v.Result.Devices, nil
}
