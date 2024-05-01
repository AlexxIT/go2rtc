package nest

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type API struct {
	Token     string
	ExpiresAt time.Time

	StreamProjectID string
	StreamDeviceID  string
	StreamSessionID string
	StreamExpiresAt time.Time

	extendTimer *time.Timer
}

type Auth struct {
	AccessToken string
}

var cache = map[string]*API{}
var cacheMu sync.Mutex

func NewAPI(clientID, clientSecret, refreshToken string) (*API, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	key := clientID + ":" + clientSecret + ":" + refreshToken
	now := time.Now()

	if api := cache[key]; api != nil && now.Before(api.ExpiresAt) {
		return api, nil
	}

	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"client_secret": []string{clientSecret},
		"refresh_token": []string{refreshToken},
	}

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.PostForm("https://www.googleapis.com/oauth2/v4/token", data)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		AccessToken string        `json:"access_token"`
		ExpiresIn   time.Duration `json:"expires_in"`
		Scope       string        `json:"scope"`
		TokenType   string        `json:"token_type"`
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return nil, err
	}

	api := &API{
		Token:     resv.AccessToken,
		ExpiresAt: now.Add(resv.ExpiresIn * time.Second),
	}

	cache[key] = api

	return api, nil
}

func (a *API) GetDevices(projectID string) (map[string]string, error) {
	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" + projectID + "/devices"
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+a.Token)

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		Devices []Device
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return nil, err
	}

	devices := map[string]string{}

	for _, device := range resv.Devices {
		if len(device.Traits.SdmDevicesTraitsCameraLiveStream.SupportedProtocols) == 0 {
			continue
		}

		if device.Traits.SdmDevicesTraitsCameraLiveStream.SupportedProtocols[0] != "WEB_RTC" {
			continue
		}

		i := strings.LastIndexByte(device.Name, '/')
		if i <= 0 {
			continue
		}

		name := device.Traits.SdmDevicesTraitsInfo.CustomName
		devices[name] = device.Name[i+1:]
	}

	return devices, nil
}

func (a *API) ExchangeSDP(projectID, deviceID, offer string) (string, error) {
	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			Offer string `json:"offerSdp"`
		} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.GenerateWebRtcStream"
	reqv.Params.Offer = offer

	b, err := json.Marshal(reqv)
	if err != nil {
		return "", err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		projectID + "/devices/" + deviceID + ":executeCommand"
	req, err := http.NewRequest("POST", uri, bytes.NewReader(b))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+a.Token)

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		Results struct {
			Answer         string    `json:"answerSdp"`
			ExpiresAt      time.Time `json:"expiresAt"`
			MediaSessionID string    `json:"mediaSessionId"`
		} `json:"results"`
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return "", err
	}

	a.StreamProjectID = projectID
	a.StreamDeviceID = deviceID
	a.StreamSessionID = resv.Results.MediaSessionID
	a.StreamExpiresAt = resv.Results.ExpiresAt

	return resv.Results.Answer, nil
}

func (a *API) ExtendStream() error {
	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			MediaSessionID string `json:"mediaSessionId"`
		} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.ExtendWebRtcStream"
	reqv.Params.MediaSessionID = a.StreamSessionID

	b, err := json.Marshal(reqv)
	if err != nil {
		return err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		a.StreamProjectID + "/devices/" + a.StreamDeviceID + ":executeCommand"
	req, err := http.NewRequest("POST", uri, bytes.NewReader(b))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+a.Token)

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		Results struct {
			ExpiresAt      time.Time `json:"expiresAt"`
			MediaSessionID string    `json:"mediaSessionId"`
		} `json:"results"`
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return err
	}

	a.StreamSessionID = resv.Results.MediaSessionID
	a.StreamExpiresAt = resv.Results.ExpiresAt

	return nil
}

type Device struct {
	Name string `json:"name"`
	Type string `json:"type"`
	//Assignee string `json:"assignee"`
	Traits struct {
		SdmDevicesTraitsInfo struct {
			CustomName string `json:"customName"`
		} `json:"sdm.devices.traits.Info"`
		SdmDevicesTraitsCameraLiveStream struct {
			VideoCodecs        []string `json:"videoCodecs"`
			AudioCodecs        []string `json:"audioCodecs"`
			SupportedProtocols []string `json:"supportedProtocols"`
		} `json:"sdm.devices.traits.CameraLiveStream"`
		//SdmDevicesTraitsCameraImage struct {
		//	MaxImageResolution struct {
		//		Width  int `json:"width"`
		//		Height int `json:"height"`
		//	} `json:"maxImageResolution"`
		//} `json:"sdm.devices.traits.CameraImage"`
		//SdmDevicesTraitsCameraPerson struct {
		//} `json:"sdm.devices.traits.CameraPerson"`
		//SdmDevicesTraitsCameraMotion struct {
		//} `json:"sdm.devices.traits.CameraMotion"`
		//SdmDevicesTraitsDoorbellChime struct {
		//} `json:"sdm.devices.traits.DoorbellChime"`
		//SdmDevicesTraitsCameraClipPreview struct {
		//} `json:"sdm.devices.traits.CameraClipPreview"`
	} `json:"traits"`
	//ParentRelations []struct {
	//	Parent      string `json:"parent"`
	//	DisplayName string `json:"displayName"`
	//} `json:"parentRelations"`
}

func (a *API) StartExtendStreamTimer() {
	// Calculate the duration until 30 seconds before the stream expires
	duration := time.Until(a.StreamExpiresAt.Add(-30 * time.Second))
	a.extendTimer = time.AfterFunc(duration, func() {
		if err := a.ExtendStream(); err != nil {
			return
		}
		duration = time.Until(a.StreamExpiresAt.Add(-30 * time.Second))
		a.extendTimer.Reset(duration)
	})

}

func (a *API) StopExtendStreamTimer() {
	if a.extendTimer == nil {
		return
	}
	a.extendTimer.Stop()
}
