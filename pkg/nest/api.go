package nest

import (
	"bytes"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// API holds the OAuth token for one set of credentials. NewAPI caches and
// shares it between every stream that uses those credentials, so nothing
// stream-specific may live here - that belongs in Stream.
type API struct {
	Token     string
	ExpiresAt time.Time
}

// Stream is the state Google hands back for one live stream on one device.
type Stream struct {
	ProjectID string
	DeviceID  string
	ExpiresAt time.Time

	// WebRTC
	MediaSessionID string

	// RTSP
	StreamToken          string
	StreamExtensionToken string
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

// StatusError is a non-200 answer from Google. Body is the (truncated)
// response body, which is the only place the actual reason is reported,
// e.g. `"status": "FAILED_PRECONDITION"` for a camera that is switched off.
type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string {
	return e.Msg
}

func newStatusError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	msg := "nest: wrong status: " + res.Status
	if s := strings.TrimSpace(string(body)); s != "" {
		msg += ": " + s
	}
	return &StatusError{Code: res.StatusCode, Msg: msg}
}

// rejectedOffer reports whether Google turned down the WebRTC offer itself
// (400 INVALID_ARGUMENT, "offerSdp contains an invalid value."). It does so
// intermittently for cameras that are streaming fine, and a fresh offer is
// accepted, so the answer is about that one request, not the device.
func rejectedOffer(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == 400 && strings.Contains(se.Msg, "INVALID_ARGUMENT")
}

// retryable reports whether a failed request may succeed if repeated soon.
// Any other 4xx that survived ExchangeSDP's own 401/409/429 handling is a
// definitive answer about the device and only wastes the retry window.
func retryable(err error) bool {
	var te *ThrottledError
	if errors.As(err, &te) {
		return false
	}
	if rejectedOffer(err) {
		return true
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code < 400 || se.Code >= 500
	}
	return true
}

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
	defer res.Body.Close()

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

func (a *API) GetDevices(projectID string) ([]DeviceInfo, error) {
	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" + projectID + "/devices"
	if err := limits.List(projectID, dialMaxWait); err != nil {
		return nil, err
	}
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

func (a *API) ExchangeSDP(projectID, deviceID, offer string) (string, *Stream, error) {
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
		return "", nil, err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		projectID + "/devices/" + deviceID + ":executeCommand"

	maxRetries := 3
	retryDelay := time.Second * 30

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := limits.Command(projectID, deviceID, reqv.Command, dialMaxWait); err != nil {
			return "", nil, err
		}

		req, err := http.NewRequest("POST", uri, bytes.NewReader(b))
		if err != nil {
			return "", nil, err
		}

		req.Header.Set("Authorization", "Bearer "+a.Token)

		client := &http.Client{Timeout: time.Second * 5000}
		res, err := client.Do(req)
		if err != nil {
			return "", nil, err
		}

		// Google says the quota is spent: hold the whole project off for a
		// minute and fail this dial now so a fallback source can take over,
		// instead of sleeping 30 s with the consumer waiting.
		if res.StatusCode == 429 {
			limits.Penalize(projectID, penaltyAfter429)
			log.Printf("nest: %s from GenerateWebRtcStream for %s, holding off for %s", res.Status, deviceID, penaltyAfter429)
			return "", nil, newStatusError(res)
		}

		// Handle 409 (Conflict) and 401 (Unauthorized)
		if res.StatusCode == 409 || res.StatusCode == 401 {
			res.Body.Close()
			if attempt < maxRetries-1 {
				log.Printf("nest: %s from GenerateWebRtcStream for %s, refreshing token and retrying in %s", res.Status, deviceID, retryDelay)
				// Get new token from Google
				if err := a.refreshToken(); err != nil {
					return "", nil, err
				}
				time.Sleep(retryDelay)
				retryDelay *= 2 // exponential backoff
				continue
			}
		}

		defer res.Body.Close()

		if res.StatusCode != 200 {
			return "", nil, newStatusError(res)
		}

		var resv struct {
			Results struct {
				Answer         string    `json:"answerSdp"`
				ExpiresAt      time.Time `json:"expiresAt"`
				MediaSessionID string    `json:"mediaSessionId"`
			} `json:"results"`
		}

		if err = json.NewDecoder(res.Body).Decode(&resv); err != nil {
			return "", nil, err
		}

		stream := &Stream{
			ProjectID:      projectID,
			DeviceID:       deviceID,
			ExpiresAt:      resv.Results.ExpiresAt,
			MediaSessionID: resv.Results.MediaSessionID,
		}

		return resv.Results.Answer, stream, nil
	}

	return "", nil, errors.New("nest: max retries exceeded")
}

func (a *API) refreshToken() error {
	// Get the cached API with matching token to get credentials
	var refreshKey string
	cacheMu.Lock()
	for key, api := range cache {
		if api.Token == a.Token {
			refreshKey = key
			break
		}
	}
	cacheMu.Unlock()

	if refreshKey == "" {
		return errors.New("nest: unable to find cached credentials")
	}

	// Parse credentials from cache key
	parts := strings.Split(refreshKey, ":")
	if len(parts) != 3 {
		return errors.New("nest: invalid cache key format")
	}
	clientID, clientSecret, refreshToken := parts[0], parts[1], parts[2]

	// Get new API instance which will refresh the token
	newAPI, err := NewAPI(clientID, clientSecret, refreshToken)
	if err != nil {
		return err
	}

	// Update current API with new token
	a.Token = newAPI.Token
	a.ExpiresAt = newAPI.ExpiresAt
	return nil
}

// ExtendStream asks Google for another five minutes and updates the stream's
// expiry and tokens in place.
func (a *API) ExtendStream(stream *Stream) error {
	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			MediaSessionID       string `json:"mediaSessionId,omitempty"`
			StreamExtensionToken string `json:"streamExtensionToken,omitempty"`
		} `json:"params"`
	}

	if stream.StreamToken != "" {
		// RTSP
		reqv.Command = "sdm.devices.commands.CameraLiveStream.ExtendRtspStream"
		reqv.Params.StreamExtensionToken = stream.StreamExtensionToken
	} else {
		// WebRTC
		reqv.Command = "sdm.devices.commands.CameraLiveStream.ExtendWebRtcStream"
		reqv.Params.MediaSessionID = stream.MediaSessionID
	}

	b, err := json.Marshal(reqv)
	if err != nil {
		return err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		stream.ProjectID + "/devices/" + stream.DeviceID + ":executeCommand"

	// a stream outlives the hour-long access token, so refresh once on 401
	for attempt := 0; ; attempt++ {
		if err := limits.CommandBackground(stream.ProjectID, stream.DeviceID, reqv.Command, extendMaxWait); err != nil {
			return err
		}
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

		if res.StatusCode == 401 && attempt == 0 {
			res.Body.Close()
			if err = a.refreshToken(); err != nil {
				return err
			}
			continue
		}

		defer res.Body.Close()

		if res.StatusCode != 200 {
			return newStatusError(res)
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
			return err
		}

		stream.ExpiresAt = resv.Results.ExpiresAt
		if resv.Results.MediaSessionID != "" {
			stream.MediaSessionID = resv.Results.MediaSessionID
		}
		if resv.Results.StreamExtensionToken != "" {
			stream.StreamExtensionToken = resv.Results.StreamExtensionToken
		}
		if resv.Results.StreamToken != "" {
			stream.StreamToken = resv.Results.StreamToken
		}

		return nil
	}
}

// keepAlive extends the stream a minute before each expiry until Stop.
// extendSpread returns a per-device offset in [0, 2 min) so that sessions
// dialed together do not extend together. Deterministic, so a reconnect
// lands the same stream in the same slot.
func extendSpread(deviceID string) time.Duration {
	h := fnv.New32a()
	h.Write([]byte(deviceID))
	return time.Duration(h.Sum32()%120) * time.Second
}

func (a *API) keepAlive(stream *Stream) *session {
	return newSession(stream.ExpiresAt, time.Minute, extendSpread(stream.DeviceID), func() (time.Time, error) {
		if err := a.ExtendStream(stream); err != nil {
			return time.Time{}, err
		}
		return stream.ExpiresAt, nil
	})
}

func (a *API) GenerateRtspStream(projectID, deviceID string) (string, *Stream, error) {
	var reqv struct {
		Command string   `json:"command"`
		Params  struct{} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.GenerateRtspStream"

	b, err := json.Marshal(reqv)
	if err != nil {
		return "", nil, err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		projectID + "/devices/" + deviceID + ":executeCommand"
	if err := limits.Command(projectID, deviceID, reqv.Command, dialMaxWait); err != nil {
		return "", nil, err
	}
	req, err := http.NewRequest("POST", uri, bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Authorization", "Bearer "+a.Token)

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", nil, newStatusError(res)
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
		return "", nil, err
	}

	if _, ok := resv.Results.StreamURLs["rtspUrl"]; !ok {
		return "", nil, errors.New("nest: failed to generate rtsp url")
	}

	stream := &Stream{
		ProjectID:            projectID,
		DeviceID:             deviceID,
		ExpiresAt:            resv.Results.ExpiresAt,
		StreamToken:          resv.Results.StreamToken,
		StreamExtensionToken: resv.Results.StreamExtensionToken,
	}

	return resv.Results.StreamURLs["rtspUrl"], stream, nil
}

func (a *API) StopRTSPStream(stream *Stream) error {
	if stream == nil || stream.ProjectID == "" || stream.DeviceID == "" {
		return errors.New("nest: tried to stop rtsp stream without a project or device ID")
	}

	var reqv struct {
		Command string `json:"command"`
		Params  struct {
			StreamExtensionToken string `json:"streamExtensionToken"`
		} `json:"params"`
	}
	reqv.Command = "sdm.devices.commands.CameraLiveStream.StopRtspStream"
	reqv.Params.StreamExtensionToken = stream.StreamExtensionToken

	b, err := json.Marshal(reqv)
	if err != nil {
		return err
	}

	uri := "https://smartdevicemanagement.googleapis.com/v1/enterprises/" +
		stream.ProjectID + "/devices/" + stream.DeviceID + ":executeCommand"
	if err := limits.CommandBackground(stream.ProjectID, stream.DeviceID, reqv.Command, extendMaxWait); err != nil {
		return err
	}
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
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return newStatusError(res)
	}

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
