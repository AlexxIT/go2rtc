package ring

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

var clientCache = map[string]*RingApi{}
var cacheMutex sync.Mutex

type RefreshTokenAuth struct {
	RefreshToken string
}

type EmailAuth struct {
	Email    string
	Password string
}

type AuthConfig struct {
	RT  string `json:"rt"`  // Refresh Token
	HID string `json:"hid"` // Hardware ID
}

type AuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`      // Always "client"
	TokenType    string `json:"token_type"` // Always "Bearer"
}

type Auth2faResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	TSVState         string `json:"tsv_state"`
	Phone            string `json:"phone"`
	NextTimeInSecs   int    `json:"next_time_in_secs"`
}

type SocketTicketResponse struct {
	Ticket            string `json:"ticket"`
	ResponseTimestamp int64  `json:"response_timestamp"`
}

type SessionResponse struct {
	Profile struct {
		ID        int64  `json:"id"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	} `json:"profile"`
}

type RingApi struct {
	httpClient     *http.Client
	authConfig     *AuthConfig
	hardwareID     string
	authToken      *AuthTokenResponse
	tokenExpiry    time.Time
	Using2FA       bool
	PromptFor2FA   string
	RefreshToken   string
	auth           interface{} // EmailAuth or RefreshTokenAuth
	onTokenRefresh func(string)
	authMutex      sync.Mutex
	session        *SessionResponse
	sessionExpiry  time.Time
	sessionMutex   sync.Mutex
	cacheKey       string
}

type CameraKind string

type CameraData struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	DeviceID    string `json:"device_id"`
	Kind        string `json:"kind"`
	LocationID  string `json:"location_id"`
}

type RingDeviceType string

type RingDevicesResponse struct {
	Doorbots           []CameraData             `json:"doorbots"`
	AuthorizedDoorbots []CameraData             `json:"authorized_doorbots"`
	StickupCams        []CameraData             `json:"stickup_cams"`
	AllCameras         []CameraData             `json:"all_cameras"`
	Chimes             []CameraData             `json:"chimes"`
	Other              []map[string]interface{} `json:"other"`
}

const (
	Doorbot             CameraKind = "doorbot"
	Doorbell            CameraKind = "doorbell"
	DoorbellV3          CameraKind = "doorbell_v3"
	DoorbellV4          CameraKind = "doorbell_v4"
	DoorbellV5          CameraKind = "doorbell_v5"
	DoorbellOyster      CameraKind = "doorbell_oyster"
	DoorbellPortal      CameraKind = "doorbell_portal"
	DoorbellScallop     CameraKind = "doorbell_scallop"
	DoorbellScallopLite CameraKind = "doorbell_scallop_lite"
	DoorbellGraham      CameraKind = "doorbell_graham_cracker"
	LpdV1               CameraKind = "lpd_v1"
	LpdV2               CameraKind = "lpd_v2"
	LpdV4               CameraKind = "lpd_v4"
	JboxV1              CameraKind = "jbox_v1"
	StickupCam          CameraKind = "stickup_cam"
	StickupCamV3        CameraKind = "stickup_cam_v3"
	StickupCamElite     CameraKind = "stickup_cam_elite"
	StickupCamLongfin   CameraKind = "stickup_cam_longfin"
	StickupCamLunar     CameraKind = "stickup_cam_lunar"
	SpotlightV2         CameraKind = "spotlightw_v2"
	HpCamV1             CameraKind = "hp_cam_v1"
	HpCamV2             CameraKind = "hp_cam_v2"
	StickupCamV4        CameraKind = "stickup_cam_v4"
	FloodlightV1        CameraKind = "floodlight_v1"
	FloodlightV2        CameraKind = "floodlight_v2"
	FloodlightPro       CameraKind = "floodlight_pro"
	CocoaCamera         CameraKind = "cocoa_camera"
	CocoaDoorbell       CameraKind = "cocoa_doorbell"
	CocoaFloodlight     CameraKind = "cocoa_floodlight"
	CocoaSpotlight      CameraKind = "cocoa_spotlight"
	StickupCamMini      CameraKind = "stickup_cam_mini"
	OnvifCamera         CameraKind = "onvif_camera"
)

const (
	IntercomHandsetAudio RingDeviceType = "intercom_handset_audio"
	OnvifCameraType      RingDeviceType = "onvif_camera"
)

const (
	clientAPIBaseURL   = "https://api.ring.com/clients_api/"
	deviceAPIBaseURL   = "https://api.ring.com/devices/v1/"
	commandsAPIBaseURL = "https://api.ring.com/commands/v1/"
	appAPIBaseURL      = "https://prd-api-us.prd.rings.solutions/api/v1/"
	oauthURL           = "https://oauth.ring.com/oauth/token"
	apiVersion         = 11
	defaultTimeout     = 20 * time.Second
	maxRetries         = 3
	sessionValidTime   = 12 * time.Hour
)

func NewRestClient(auth interface{}, onTokenRefresh func(string)) (*RingApi, error) {
	var cacheKey string

	// Create cache key based on auth data
	switch a := auth.(type) {
	case RefreshTokenAuth:
		if a.RefreshToken == "" {
			return nil, fmt.Errorf("refresh token is required")
		}
		cacheKey = "refresh:" + a.RefreshToken
	case EmailAuth:
		if a.Email == "" || a.Password == "" {
			return nil, fmt.Errorf("email and password are required")
		}
		cacheKey = "email:" + a.Email + ":" + a.Password
	default:
		return nil, fmt.Errorf("invalid auth type")
	}

	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if cachedClient, ok := clientCache[cacheKey]; ok {
		// Check if token is not nil and not expired
		if cachedClient.authToken != nil && time.Now().Before(cachedClient.tokenExpiry) {
			cachedClient.onTokenRefresh = onTokenRefresh
			return cachedClient, nil
		}
	}

	client := &RingApi{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		onTokenRefresh: onTokenRefresh,
		hardwareID:     generateHardwareID(),
		auth:           auth,
		cacheKey:       cacheKey,
	}

	switch a := auth.(type) {
	case RefreshTokenAuth:
		config, err := parseAuthConfig(a.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("failed to parse refresh token: %w", err)
		}

		client.authConfig = config
		client.hardwareID = config.HID
		client.RefreshToken = a.RefreshToken
	}

	clientCache[cacheKey] = client

	return client, nil
}

func ClientAPI(path string) string {
	return clientAPIBaseURL + path
}

func DeviceAPI(path string) string {
	return deviceAPIBaseURL + path
}

func CommandsAPI(path string) string {
	return commandsAPIBaseURL + path
}

func AppAPI(path string) string {
	return appAPIBaseURL + path
}

func (c *RingApi) GetAuth(twoFactorAuthCode string) (*AuthTokenResponse, error) {
	var grantData map[string]string

	if c.authConfig != nil && twoFactorAuthCode == "" {
		grantData = map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": c.authConfig.RT,
		}
	} else {
		authEmail, ok := c.auth.(EmailAuth)
		if !ok {
			return nil, fmt.Errorf("invalid auth type for email authentication")
		}
		grantData = map[string]string{
			"grant_type": "password",
			"username":   authEmail.Email,
			"password":   authEmail.Password,
		}
	}

	grantData["client_id"] = "ring_official_android"
	grantData["scope"] = "client"

	body, err := json.Marshal(grantData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequest("POST", oauthURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("hardware_id", c.hardwareID)
	req.Header.Set("User-Agent", "android:com.ringapp")
	req.Header.Set("2fa-support", "true")
	if twoFactorAuthCode != "" {
		req.Header.Set("2fa-code", twoFactorAuthCode)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Handle 2FA Responses
	if resp.StatusCode == http.StatusPreconditionFailed ||
		(resp.StatusCode == http.StatusBadRequest && strings.Contains(resp.Header.Get("WWW-Authenticate"), "Verification Code")) {

		var tfaResp Auth2faResponse
		if err := json.NewDecoder(resp.Body).Decode(&tfaResp); err != nil {
			return nil, err
		}

		c.Using2FA = true
		if resp.StatusCode == http.StatusBadRequest {
			c.PromptFor2FA = "Invalid 2fa code entered. Please try again."
			return nil, fmt.Errorf("invalid 2FA code")
		}

		if tfaResp.TSVState != "" {
			prompt := "from your authenticator app"
			if tfaResp.TSVState != "totp" {
				prompt = fmt.Sprintf("sent to %s via %s", tfaResp.Phone, tfaResp.TSVState)
			}
			c.PromptFor2FA = fmt.Sprintf("Please enter the code %s", prompt)
		} else {
			c.PromptFor2FA = "Please enter the code sent to your text/email"
		}

		return nil, fmt.Errorf("2FA required")
	}

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("auth request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp AuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Refresh token and expiry
	c.authToken = &authResp
	c.authConfig = &AuthConfig{
		RT:  authResp.RefreshToken,
		HID: c.hardwareID,
	}
	// Set token expiry (1 minute before actual expiry)
	expiresIn := time.Duration(authResp.ExpiresIn-60) * time.Second
	c.tokenExpiry = time.Now().Add(expiresIn)

	c.RefreshToken = encodeAuthConfig(c.authConfig)
	if c.onTokenRefresh != nil {
		c.onTokenRefresh(c.RefreshToken)
	}

	// Refresh the cached client
	cacheMutex.Lock()
	clientCache[c.cacheKey] = c
	cacheMutex.Unlock()

	return c.authToken, nil
}

func (c *RingApi) FetchRingDevices() (*RingDevicesResponse, error) {
	response, err := c.Request("GET", ClientAPI("ring_devices"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ring devices: %w", err)
	}

	var devices RingDevicesResponse
	if err := json.Unmarshal(response, &devices); err != nil {
		return nil, fmt.Errorf("failed to unmarshal devices response: %w", err)
	}

	// Process "other" devices
	var onvifCameras []CameraData
	var intercoms []CameraData

	for _, device := range devices.Other {
		kind, ok := device["kind"].(string)
		if !ok {
			continue
		}

		switch RingDeviceType(kind) {
		case OnvifCameraType:
			var camera CameraData
			if deviceJson, err := json.Marshal(device); err == nil {
				if err := json.Unmarshal(deviceJson, &camera); err == nil {
					onvifCameras = append(onvifCameras, camera)
				}
			}
		case IntercomHandsetAudio:
			var intercom CameraData
			if deviceJson, err := json.Marshal(device); err == nil {
				if err := json.Unmarshal(deviceJson, &intercom); err == nil {
					intercoms = append(intercoms, intercom)
				}
			}
		}
	}

	// Combine all cameras into AllCameras slice
	allCameras := make([]CameraData, 0)
	allCameras = append(allCameras, interfaceSlice(devices.Doorbots)...)
	allCameras = append(allCameras, interfaceSlice(devices.StickupCams)...)
	allCameras = append(allCameras, interfaceSlice(devices.AuthorizedDoorbots)...)
	allCameras = append(allCameras, interfaceSlice(onvifCameras)...)
	allCameras = append(allCameras, interfaceSlice(intercoms)...)

	devices.AllCameras = allCameras

	return &devices, nil
}

func (c *RingApi) GetSocketTicket() (*SocketTicketResponse, error) {
	response, err := c.Request("POST", AppAPI("clap/ticket/request/signalsocket"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch socket ticket: %w", err)
	}

	var ticket SocketTicketResponse
	if err := json.Unmarshal(response, &ticket); err != nil {
		return nil, fmt.Errorf("failed to unmarshal socket ticket response: %w", err)
	}

	return &ticket, nil
}

func (c *RingApi) Request(method, url string, body interface{}) ([]byte, error) {
	// Ensure we have a valid session
	if err := c.ensureSession(); err != nil {
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	// Create request
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.authToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("hardware_id", c.hardwareID)
	req.Header.Set("User-Agent", "android:com.ringapp")

	// Make request with retries
	var resp *http.Response
	var responseBody []byte

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		defer resp.Body.Close()

		responseBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Handle 401 by refreshing auth and retrying
		if resp.StatusCode == http.StatusUnauthorized {
			// Reset token to force refresh
			c.authMutex.Lock()
			c.authToken = nil
			c.tokenExpiry = time.Time{} // Reset token expiry
			c.authMutex.Unlock()

			if attempt == maxRetries {
				return nil, fmt.Errorf("authentication failed after %d retries", maxRetries)
			}

			// By 401 with Auth AND Session start over
			c.sessionMutex.Lock()
			c.session = nil
			c.sessionExpiry = time.Time{} // Reset session expiry
			c.sessionMutex.Unlock()

			if err := c.ensureSession(); err != nil {
				return nil, fmt.Errorf("failed to refresh session: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+c.authToken.AccessToken)
			continue
		}

		// Handle 404 error with hardware_id reference - session issue
		if resp.StatusCode == 404 && strings.Contains(url, clientAPIBaseURL) {
			var errorBody map[string]interface{}
			if err := json.Unmarshal(responseBody, &errorBody); err == nil {
				if errorStr, ok := errorBody["error"].(string); ok && strings.Contains(errorStr, c.hardwareID) {
					// Session with hardware_id not found, refresh session
					c.sessionMutex.Lock()
					c.session = nil
					c.sessionExpiry = time.Time{} // Reset session expiry
					c.sessionMutex.Unlock()

					if attempt == maxRetries {
						return nil, fmt.Errorf("session refresh failed after %d retries", maxRetries)
					}

					if err := c.ensureSession(); err != nil {
						return nil, fmt.Errorf("failed to refresh session: %w", err)
					}

					continue
				}
			}
		}

		// Handle other error status codes
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
		}

		break
	}

	return responseBody, nil
}

func (c *RingApi) ensureSession() error {
	c.sessionMutex.Lock()
	defer c.sessionMutex.Unlock()

	// If session is still valid, use it
	if c.session != nil && time.Now().Before(c.sessionExpiry) {
		return nil
	}

	// Make sure we have a valid auth token
	if err := c.ensureAuth(); err != nil {
		return fmt.Errorf("authentication failed while creating session: %w", err)
	}

	sessionPayload := map[string]interface{}{
		"device": map[string]interface{}{
			"hardware_id": c.hardwareID,
			"metadata": map[string]interface{}{
				"api_version":  apiVersion,
				"device_model": "ring-client-go",
			},
			"os": "android",
		},
	}

	body, err := json.Marshal(sessionPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal session request: %w", err)
	}

	req, err := http.NewRequest("POST", ClientAPI("session"), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken.AccessToken)
	req.Header.Set("hardware_id", c.hardwareID)
	req.Header.Set("User-Agent", "android:com.ringapp")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var sessionResp SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return fmt.Errorf("failed to decode session response: %w", err)
	}

	c.session = &sessionResp
	c.sessionExpiry = time.Now().Add(sessionValidTime)

	// Aktualisiere den gecachten Client
	cacheMutex.Lock()
	clientCache[c.cacheKey] = c
	cacheMutex.Unlock()

	return nil
}

func (c *RingApi) ensureAuth() error {
	c.authMutex.Lock()
	defer c.authMutex.Unlock()

	// If token exists and is not expired, use it
	if c.authToken != nil && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	var grantData = map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.authConfig.RT,
	}

	// Add common fields
	grantData["client_id"] = "ring_official_android"
	grantData["scope"] = "client"

	// Make auth request
	body, err := json.Marshal(grantData)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequest("POST", oauthURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("hardware_id", c.hardwareID)
	req.Header.Set("User-Agent", "android:com.ringapp")
	req.Header.Set("2fa-support", "true")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return fmt.Errorf("2FA required. Please see documentation for handling 2FA")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp AuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Update auth config and refresh token
	c.authToken = &authResp
	c.authConfig = &AuthConfig{
		RT:  authResp.RefreshToken,
		HID: c.hardwareID,
	}

	// Set token expiry (1 minute before actual expiry)
	expiresIn := time.Duration(authResp.ExpiresIn-60) * time.Second
	c.tokenExpiry = time.Now().Add(expiresIn)

	// Encode and notify about new refresh token
	if c.onTokenRefresh != nil {
		newRefreshToken := encodeAuthConfig(c.authConfig)
		c.onTokenRefresh(newRefreshToken)
	}

	// Refreshn the token in the client
	c.RefreshToken = encodeAuthConfig(c.authConfig)

	// Refresh the cached client
	cacheMutex.Lock()
	clientCache[c.cacheKey] = c
	cacheMutex.Unlock()

	return nil
}

func parseAuthConfig(refreshToken string) (*AuthConfig, error) {
	decoded, err := base64.StdEncoding.DecodeString(refreshToken)
	if err != nil {
		return nil, err
	}

	var config AuthConfig
	if err := json.Unmarshal(decoded, &config); err != nil {
		// Handle legacy format where refresh token is the raw token
		return &AuthConfig{RT: refreshToken}, nil
	}

	return &config, nil
}

func encodeAuthConfig(config *AuthConfig) string {
	jsonBytes, _ := json.Marshal(config)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

func generateHardwareID() string {
	h := sha256.New()
	h.Write([]byte("ring-client-go2rtc"))
	return hex.EncodeToString(h.Sum(nil)[:16])
}

func interfaceSlice(slice interface{}) []CameraData {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		return nil
	}

	ret := make([]CameraData, s.Len())
	for i := 0; i < s.Len(); i++ {
		if camera, ok := s.Index(i).Interface().(CameraData); ok {
			ret[i] = camera
		}
	}
	return ret
}
