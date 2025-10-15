package yandex

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Session struct {
	token  string
	client *http.Client
}

var sessions = map[string]*Session{}
var sessionsMu sync.Mutex

func GetSession(token string) (*Session, error) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if session, ok := sessions[token]; ok {
		return session, nil
	}

	session := &Session{token: token}
	if err := session.Login(); err != nil {
		return nil, err
	}

	sessions[token] = session

	return session, nil
}

func (s *Session) Login() error {
	req, err := http.NewRequest(
		"POST", "https://mobileproxy.passport.yandex.net/1/bundle/auth/x_token/",
		strings.NewReader("type=x-token&retpath=https%3A%2F%2Fwww.yandex.ru"),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Ya-Consumer-Authorization", "OAuth "+s.token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	var auth struct {
		PassportHost string `json:"passport_host"`
		Status       string `json:"status"`
		TrackId      string `json:"track_id"`
	}
	if err = json.NewDecoder(res.Body).Decode(&auth); err != nil {
		return err
	}

	if auth.Status != "ok" {
		return errors.New("yandex: login error: " + auth.Status)
	}

	s.client = &http.Client{Timeout: 15 * time.Second}
	s.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	s.client.Jar, _ = cookiejar.New(nil)

	res, err = s.client.Get(auth.PassportHost + "/auth/session/?track_id=" + auth.TrackId)
	if err != nil {
		return err
	}

	s.client.CheckRedirect = nil

	return nil
}

func (s *Session) Get(url string) (*http.Response, error) {
	return s.client.Get(url)
}

func (s *Session) GetCSRF() (string, error) {
	res, err := s.Get("https://yandex.ru/quasar")
	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	token := core.Between(string(body), `"csrfToken2":"`, `"`)
	return token, nil
}

func (s *Session) GetCookieString(url string) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	for _, cookie := range s.client.Jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	return req.Header.Get("Cookie")
}

func (s *Session) GetDevices() ([]Device, error) {
	res, err := s.Get("https://iot.quasar.yandex.ru/m/v3/user/devices")
	if err != nil {
		return nil, err
	}

	var data struct {
		Households []struct {
			All []Device `json:"all"`
		} `json:"households"`
	}

	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}

	var devices []Device
	for _, household := range data.Households {
		devices = append(devices, household.All...)
	}
	return devices, nil
}

func (s *Session) GetSnapshotURL(deviceID string) (string, error) {
	devices, err := s.GetDevices()
	if err != nil {
		return "", err
	}

	for _, device := range devices {
		if device.Id == deviceID {
			return device.Parameters.SnapshotUrl, nil
		}
	}

	return "", errors.New("yandex: can't get snapshot url for device: " + deviceID)
}

func (s *Session) WebrtcCreateRoom(deviceID string) (*Room, error) {
	csrf, err := s.GetCSRF()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST", "https://iot.quasar.yandex.ru/m/v3/user/devices/"+deviceID+"/webrtc/create-room",
		strings.NewReader(`{"protocol":"whip"}`),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-CSRF-Token", csrf)

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	var data struct {
		Result Room `json:"result"`
	}
	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data.Result, nil
}

type Device struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Parameters struct {
		SnapshotUrl string `json:"snapshot_url,omitempty"`
	} `json:"parameters"`
}

type Room struct {
	ServiceUrl    string `json:"service_url"`
	ServiceName   string `json:"service_name"`
	RoomId        string `json:"room_id"`
	ParticipantId string `json:"participant_id"`
	Credentials   string `json:"jwt"`
}
