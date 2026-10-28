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
	Token        string
	ExpiresAt    time.Time
	ClientID     string
	ClientSecret string
	RefreshToken string

	StreamProjectID string
	StreamDeviceID  string
	StreamExpiresAt time.Time

	// WebRTC
	StreamSessionID string

	// RTSP
	StreamToken          string
	StreamExtensionToken string

	extendTimer *time.Timer
	extendMu    sync.Mutex
	extendStop  chan struct{}
}

type Auth struct {
	AccessToken string
}

type DeviceInfo struct {
	Name      string
	DeviceID  string
	Protocols []string
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

	token, expiresAt, err := requestAccessToken(clientID, clientSecret, refreshToken)
	if err != nil {
		return nil, err
	}

	api := &API{
		Token:        token,
		ExpiresAt:    expiresAt,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
	}

	cache[key] = api

	return api, nil
}

func requestAccessToken(clientID, clientSecret, refreshToken string) (string, time.Time, error) {
	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"client_id":     []string{clientID},
		"client_secret": []string{clientSecret},
		"refresh_token": []string{refreshToken},
	}

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.PostForm("https://www.googleapis.com/oauth2/v4/token", data)
	if err != nil {
		return "", time.Time{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", time.Time{}, errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		AccessToken string        `json:"access_token"`
		ExpiresIn   time.Duration `json:"expires_in"`
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	return resv.AccessToken, now.Add(resv.ExpiresIn * time.Second), nil
}

func (a *API) GetDevices(projectID string) ([]DeviceInfo, error) {
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
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, errors.New("nest: wrong status: " + res.Status)
	}

	var resv struct {
		Devices []Device
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return nil, err
	}

	devices := make([]DeviceInfo, 0, len(resv.Devices))

	for _, device := range resv.Devices {
		// only RTSP and WEB_RTC available (both supported)
		if len(device.Traits.SdmDevicesTraitsCameraLiveStream.SupportedProtocols) == 0 {
			continue
		}

		i := strings.LastIndexByte(device.Name, '/')
		if i <= 0 {
			continue
		}

		name := device.Traits.SdmDevicesTraitsInfo.CustomName
		// Devices configured through the Nest app use the container/room name as opposed to the customName trait
		if name == "" && len(device.ParentRelations) > 0 {
			name = device.ParentRelations[0].DisplayName
		}

		devices = append(devices, DeviceInfo{
			Name:      name,
			DeviceID:  device.Name[i+1:],
			Protocols: device.Traits.SdmDevicesTraitsCameraLiveStream.SupportedProtocols,
		})
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

	maxRetries := 3
	retryDelay := time.Second * 30

	for attempt := 0; attempt < maxRetries; attempt++ {
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

		// Handle 409 (Conflict), 429 (Too Many Requests), and 401 (Unauthorized)
		if res.StatusCode == 409 || res.StatusCode == 429 || res.StatusCode == 401 {
			res.Body.Close()
			if attempt < maxRetries-1 {
				// Get new token from Google
				if err := a.refreshToken(); err != nil {
					return "", err
				}
				time.Sleep(retryDelay)
				retryDelay *= 2 // exponential backoff
				continue
			}
		}

		defer res.Body.Close()

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

	return "", errors.New("nest: max retries exceeded")
}

func (a *API) refreshToken() error {
	clientID := a.ClientID
	clientSecret := a.ClientSecret
	refreshToken := a.RefreshToken

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		// Backward-compatible fallback: derive credentials from cache key.
		var refreshKey string
		cacheMu.Lock()
		for key, api := range cache {
			if api == a || api.Token == a.Token {
				refreshKey = key
				break
			}
		}
		cacheMu.Unlock()

		if refreshKey == "" {
			return errors.New("nest: unable to find cached credentials")
		}

		parts := strings.Split(refreshKey, ":")
		if len(parts) != 3 {
			return errors.New("nest: invalid cache key format")
		}
		clientID, clientSecret, refreshToken = parts[0], parts[1], parts[2]
	}

	token, expiresAt, err := requestAccessToken(clientID, clientSecret, refreshToken)
	if err != nil {
		return err
	}

	a.Token = token
	a.ExpiresAt = expiresAt
	a.ClientID = clientID
	a.ClientSecret = clientSecret
	a.RefreshToken = refreshToken

	cacheMu.Lock()
	cache[clientID+":"+clientSecret+":"+refreshToken] = a
	cacheMu.Unlock()
	return nil
}

func (a *API) ExtendStream() error {
	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			MediaSessionID       string `json:"mediaSessionId,omitempty"`
			StreamExtensionToken string `json:"streamExtensionToken,omitempty"`
		} `json:"params"`
	}

	if a.StreamToken != "" {
		// RTSP
		reqv.Command = "sdm.devices.commands.CameraLiveStream.ExtendRtspStream"
		reqv.Params.StreamExtensionToken = a.StreamExtensionToken
	} else {
		// WebRTC
		reqv.Command = "sdm.devices.commands.CameraLiveStream.ExtendWebRtcStream"
		reqv.Params.MediaSessionID = a.StreamSessionID
	}

	b, err := json.Marshal(reqv)
	if err != nil {
		return err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		a.StreamProjectID + "/devices/" + a.StreamDeviceID + ":executeCommand"

	maxRetries := 3
	retryDelay := 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
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

		// 401 => force refresh token and retry fast
		if res.StatusCode == 401 {
			res.Body.Close()
			if attempt < maxRetries-1 {
				if err := a.refreshToken(); err != nil {
					return err
				}
				time.Sleep(time.Second)
				continue
			}
		}

		// 409/429 => backoff en retry (zonder meteen token refresh)
		if res.StatusCode == 409 || res.StatusCode == 429 {
			res.Body.Close()
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}
		}

		if res.StatusCode != 200 {
			res.Body.Close()
			return errors.New("nest: wrong status: " + res.Status)
		}

		var resv struct {
			Results struct {
				ExpiresAt            time.Time `json:"expiresAt"`
				MediaSessionID       string    `json:"mediaSessionId"`
				StreamExtensionToken string    `json:"streamExtensionToken"`
				StreamToken          string    `json:"streamToken"`
			} `json:"results"`
		}

		if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
			res.Body.Close()
			return err
		}
		res.Body.Close()

		a.StreamSessionID = resv.Results.MediaSessionID
		a.StreamExpiresAt = resv.Results.ExpiresAt
		a.StreamExtensionToken = resv.Results.StreamExtensionToken
		a.StreamToken = resv.Results.StreamToken

		return nil
	}

	return errors.New("nest: max retries exceeded")
}

func (a *API) GenerateRtspStream(projectID, deviceID string) (string, error) {
	var reqv struct {
		Command string   `json:"command"`
		Params  struct{} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.GenerateRtspStream"

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
			StreamURLs           map[string]string `json:"streamUrls"`
			StreamExtensionToken string            `json:"streamExtensionToken"`
			StreamToken          string            `json:"streamToken"`
			ExpiresAt            time.Time         `json:"expiresAt"`
		} `json:"results"`
	}

	if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
		return "", err
	}

	if _, ok := resv.Results.StreamURLs["rtspUrl"]; !ok {
		return "", errors.New("nest: failed to generate rtsp url")
	}

	a.StreamProjectID = projectID
	a.StreamDeviceID = deviceID
	a.StreamToken = resv.Results.StreamToken
	a.StreamExtensionToken = resv.Results.StreamExtensionToken
	a.StreamExpiresAt = resv.Results.ExpiresAt

	return resv.Results.StreamURLs["rtspUrl"], nil
}

func (a *API) StopRTSPStream() error {
	if a.StreamProjectID == "" || a.StreamDeviceID == "" {
		return errors.New("nest: tried to stop rtsp stream without a project or device ID")
	}

	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			StreamExtensionToken string `json:"streamExtensionToken"`
		} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.StopRtspStream"
	reqv.Params.StreamExtensionToken = a.StreamExtensionToken

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

	a.StreamProjectID = ""
	a.StreamDeviceID = ""
	a.StreamExtensionToken = ""
	a.StreamToken = ""

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
	ParentRelations []struct {
		Parent      string `json:"parent"`
		DisplayName string `json:"displayName"`
	} `json:"parentRelations"`
}

func (a *API) StartExtendStreamTimer() {
	a.extendMu.Lock()
	if a.extendStop != nil {
		a.extendMu.Unlock()
		return
	}
	a.extendStop = make(chan struct{})
	stop := a.extendStop
	a.extendMu.Unlock()

	go func() {
		for {
			// plan 1 minuut vóór expiry; clamp zodat het nooit negatief/te klein wordt
			d := time.Until(a.StreamExpiresAt) - time.Minute
			if d < 10*time.Second {
				d = 10 * time.Second
			}

			t := time.NewTimer(d)
			select {
			case <-t.C:
				// Keep retrying extension on transient failures to avoid avoidable stream drops.
				if err := a.ExtendStream(); err != nil {
					continue
				}
				// loop gaat door en plant opnieuw met nieuw StreamExpiresAt
			case <-stop:
				t.Stop()
				return
			}
		}
	}()
}

func (a *API) StopExtendStreamTimer() {
	a.extendMu.Lock()
	if a.extendStop != nil {
		close(a.extendStop)
		a.extendStop = nil
	}
	a.extendMu.Unlock()
}
