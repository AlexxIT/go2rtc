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
	"time"
)

type RefreshTokenAuth struct {
	RefreshToken string
}

// AuthConfig represents the decoded refresh token data
type AuthConfig struct {
	RT  string `json:"rt"`  // Refresh Token
	HID string `json:"hid"` // Hardware ID
}

// AuthTokenResponse represents the response from the authentication endpoint
type AuthTokenResponse struct {
    AccessToken		string `json:"access_token"`
    ExpiresIn		int    `json:"expires_in"`
    RefreshToken 	string `json:"refresh_token"`
    Scope       	string `json:"scope"`     // Always "client"
    TokenType   	string `json:"token_type"` // Always "Bearer"
}

// SocketTicketRequest represents the request to get a socket ticket
type SocketTicketResponse struct {
	Ticket 				string 	`json:"ticket"`
	ResponseTimestamp 	int64 	`json:"response_timestamp"`
}

// RingRestClient handles authentication and requests to Ring API
type RingRestClient struct {
	httpClient       *http.Client
	authConfig       *AuthConfig
	hardwareID       string
	authToken        *AuthTokenResponse
	auth           	 RefreshTokenAuth
	onTokenRefresh   func(string) // Callback when refresh token is updated
}

// CameraKind represents the different types of Ring cameras
type CameraKind string

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

// RingDeviceType represents different types of Ring devices
type RingDeviceType string

const (
	IntercomHandsetAudio RingDeviceType = "intercom_handset_audio"
	OnvifCameraType      RingDeviceType = "onvif_camera"
)

// CameraData contains common fields for all camera types
type CameraData struct {
	ID          float64	`json:"id"`
	Description string 	`json:"description"`
	DeviceID    string 	`json:"device_id"`
	Kind        string 	`json:"kind"`
	LocationID  string 	`json:"location_id"`
}

// RingDevicesResponse represents the response from the Ring API
type RingDevicesResponse struct {
	Doorbots           []CameraData              `json:"doorbots"`
	AuthorizedDoorbots []CameraData              `json:"authorized_doorbots"`
	StickupCams        []CameraData              `json:"stickup_cams"`
	AllCameras         []CameraData              `json:"all_cameras"`
	Chimes             []CameraData              `json:"chimes"`
	Other              []map[string]interface{}  `json:"other"`
}

const (
	clientAPIBaseURL    = "https://api.ring.com/clients_api/"
	deviceAPIBaseURL    = "https://api.ring.com/devices/v1/"
	commandsAPIBaseURL  = "https://api.ring.com/commands/v1/"
	appAPIBaseURL       = "https://prd-api-us.prd.rings.solutions/api/v1/"
	oauthURL           	= "https://oauth.ring.com/oauth/token"
	apiVersion         	= 11
	defaultTimeout     	= 20 * time.Second
	maxRetries        	= 3
)

// NewRingRestClient creates a new Ring client instance
func NewRingRestClient(auth RefreshTokenAuth, onTokenRefresh func(string)) (*RingRestClient, error) {
	client := &RingRestClient{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		auth:          	auth,
		onTokenRefresh: onTokenRefresh,
		hardwareID:   	generateHardwareID(),
	}

	// check if refresh token is provided
	if auth.RefreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	if config, err := parseAuthConfig(auth.RefreshToken); err == nil {
		client.authConfig = config
		client.hardwareID = config.HID
	}

	return client, nil
}

// Request makes an authenticated request to the Ring API
func (c *RingRestClient) Request(method, url string, body interface{}) ([]byte, error) {
	// Ensure we have a valid auth token
	if err := c.ensureAuth(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
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
			c.authToken = nil // Force token refresh
			if attempt == maxRetries {
				return nil, fmt.Errorf("authentication failed after %d retries", maxRetries)
			}
			if err := c.ensureAuth(); err != nil {
				return nil, fmt.Errorf("failed to refresh authentication: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+c.authToken.AccessToken)
			continue
		}

		// Handle other error status codes
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
		}

		break
	}

	return responseBody, nil
}

// ensureAuth ensures we have a valid auth token
func (c *RingRestClient) ensureAuth() error {
	if c.authToken != nil {
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

	// Encode and notify about new refresh token
	if c.onTokenRefresh != nil {
		newRefreshToken := encodeAuthConfig(c.authConfig)
		c.onTokenRefresh(newRefreshToken)
	}

	return nil
}

// Helper functions for auth config encoding/decoding
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

// API URL helpers
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

// FetchRingDevices gets all Ring devices and categorizes them
func (c *RingRestClient) FetchRingDevices() (*RingDevicesResponse, error) {
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

func (c *RingRestClient) GetSocketTicket() (*SocketTicketResponse, error) {
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