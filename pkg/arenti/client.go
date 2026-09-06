package arenti

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURLEU  = "https://web-eu.arenti.net"
	DefaultBaseURLUS  = "https://web-us.arenti.net"
	DefaultBaseURL    = DefaultBaseURLEU
	DefaultSourceApp  = "39"
	MeariPasswordSalt = "https://www.mearitek.com/zh/home-cn/"
	MeariRSAPublicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCS3LSuG7ttWGvFV+Cn6FCKqqMxe9kF81McQO+tc4H5n1FImoeDDM28z1mnGGSqJHNAUbiRzcHYL8VAblH7Lbo7SwDnQtm+gjRIl9yUuyLBlA39ry14+dqCEXaO9N4hNOeRbZUXxTB126DkvKQOxzfoU1/mnDji0gUCy/zcB1KNOwIDAQAB
-----END PUBLIC KEY-----`
)

var americasCountries = map[string]bool{
	"US": true, "CA": true, "MX": true, "BR": true, "AR": true, "CL": true,
	"CO": true, "PE": true, "VE": true, "EC": true, "GT": true, "CU": true,
	"BO": true, "DO": true, "HN": true, "PY": true, "SV": true, "NI": true,
	"CR": true, "PA": true, "UY": true, "JM": true, "TT": true, "PR": true,
	"VI": true, "BS": true, "BZ": true, "GY": true, "SR": true,
}

// ResolveBaseURL determines the appropriate regional REST API endpoint.
func ResolveBaseURL(countryCode, region, server string) string {
	if server != "" {
		return strings.TrimRight(server, "/")
	}
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "us", "usa", "america", "americas":
		return DefaultBaseURLUS
	case "eu", "europe":
		return DefaultBaseURLEU
	}
	if americasCountries[strings.ToUpper(strings.TrimSpace(countryCode))] {
		return DefaultBaseURLUS
	}
	return DefaultBaseURLEU
}

type Client struct {
	Account     string
	Password    string
	CountryCode string
	PhoneCode   string
	SourceApp   string
	BaseURL     string
	UserID      int64
	UserToken   string
	WssDomain   string
	Battery     *bool

	httpClient *http.Client
	mu         sync.RWMutex
}

func NewClient(account, password, countryCode string) *Client {
	if countryCode == "" {
		countryCode = "US"
	}
	cc := strings.ToUpper(countryCode)
	return &Client{
		Account:     account,
		Password:    password,
		CountryCode: cc,
		SourceApp:   DefaultSourceApp,
		BaseURL:     ResolveBaseURL(cc, "", ""),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) SetBattery(battery bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Battery = &battery
}

func (c *Client) SetRegion(region string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BaseURL = ResolveBaseURL(c.CountryCode, region, "")
}

func (c *Client) SetBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BaseURL = strings.TrimRight(baseURL, "/")
}

// EncryptPassword performs RSA 1024-bit PKCS#1 v1.5 encryption on the salted password
// and encodes it into base64 twice as required by the Meari/Arenti cloud.
func EncryptPassword(password string) (string, error) {
	block, _ := pem.Decode([]byte(MeariRSAPublicKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode Meari RSA public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse RSA public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("public key is not an RSA public key")
	}

	salted := []byte(password + MeariPasswordSalt)
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, salted)
	if err != nil {
		return "", fmt.Errorf("RSA encryption failed: %w", err)
	}

	firstB64 := base64.StdEncoding.EncodeToString(encrypted)
	secondB64 := base64.StdEncoding.EncodeToString([]byte(firstB64))
	return secondB64, nil
}

// CalcSign calculates the HMAC-SHA256 signature for Arenti REST API requests.
func CalcSign(identity, t, nonce, queryParams, body, key string) string {
	bodyMD5 := ""
	if body != "" {
		h := md5.Sum([]byte(body))
		bodyMD5 = hex.EncodeToString(h[:])
	}
	plain := identity + t + nonce + queryParams + bodyMD5
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(plain))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

// NewNonce generates a 16-byte random hex string
func NewNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewUUID generates an RFC4122 v4 UUID string
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	encPassword, err := EncryptPassword(c.Password)
	if err != nil {
		return fmt.Errorf("arenti: password encryption failed: %w", err)
	}

	reqBody := map[string]interface{}{
		"app":         c.SourceApp,
		"countryCode": c.CountryCode,
		"lngType":     "en",
		"password":    encPassword,
		"phoneCode":   c.PhoneCode,
		"userAccount": c.Account,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	t := fmt.Sprintf("%d", time.Now().UnixMilli())
	nonce := NewNonce()
	sign := CalcSign("-1", t, nonce, "", string(bodyBytes), "-")

	loginURL := fmt.Sprintf("%s/ipc_web/user/login", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, loginURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("identity", "-1")
	req.Header.Set("t", t)
	req.Header.Set("nonce", nonce)
	req.Header.Set("sign", sign)
	req.Header.Set("signVer", "1.1")
	req.Header.Set("app", c.SourceApp)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("arenti: login request failed: %w", err)
	}
	defer resp.Body.Close()

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("arenti: failed to decode login response: %w", err)
	}

	if loginResp.Code != "1001" {
		return fmt.Errorf("arenti: login returned error code %s: %s", loginResp.Code, loginResp.Msg)
	}

	c.UserID = loginResp.Data.UserID
	c.UserToken = loginResp.Data.UserToken
	c.WssDomain = loginResp.Data.WssDomain
	if loginResp.Data.CountryCode != "" {
		c.CountryCode = loginResp.Data.CountryCode
	}
	if loginResp.Data.PhoneCode != "" {
		c.PhoneCode = loginResp.Data.PhoneCode
	}

	return nil
}

func (c *Client) EnsureLogin() error {
	c.mu.RLock()
	hasToken := c.UserToken != ""
	c.mu.RUnlock()

	if hasToken {
		return nil
	}
	return c.Login()
}

func (c *Client) doAuthRequest(method, path string, rawQuery string, bodyData []byte) ([]byte, error) {
	if err := c.EnsureLogin(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	userIDStr := fmt.Sprintf("%d", c.UserID)
	userToken := c.UserToken
	baseURL := c.BaseURL
	appID := c.SourceApp
	c.mu.RUnlock()

	bodyStr := ""
	if len(bodyData) > 0 {
		bodyStr = string(bodyData)
	}

	t := fmt.Sprintf("%d", time.Now().UnixMilli())
	nonce := NewNonce()
	sign := CalcSign(userIDStr, t, nonce, rawQuery, bodyStr, userToken)

	reqURL := fmt.Sprintf("%s%s", baseURL, path)
	if rawQuery != "" {
		reqURL += "?" + rawQuery
	}

	var bodyReader io.Reader
	if len(bodyData) > 0 {
		bodyReader = bytes.NewReader(bodyData)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("identity", userIDStr)
	req.Header.Set("t", t)
	req.Header.Set("nonce", nonce)
	req.Header.Set("sign", sign)
	req.Header.Set("signVer", "1.1")
	req.Header.Set("app", appID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// GetDevices returns all discovered cameras from the user's account.
func (c *Client) GetDevices() ([]Device, error) {
	respBytes, err := c.doAuthRequest(http.MethodGet, "/ipc_web/device/list", "", nil)
	if err != nil {
		return nil, err
	}

	var devListResp DeviceListResponse
	if err := json.Unmarshal(respBytes, &devListResp); err != nil {
		return nil, fmt.Errorf("arenti: decode device list failed: %w", err)
	}

	if devListResp.Code != "1001" {
		return nil, fmt.Errorf("arenti: device list error code %s: %s", devListResp.Code, devListResp.Msg)
	}

	var allDevices []Device
	allDevices = append(allDevices, devListResp.Data.Snap...)
	allDevices = append(allDevices, devListResp.Data.Ipc...)
	allDevices = append(allDevices, devListResp.Data.Nvr...)

	var wg sync.WaitGroup
	for i := range allDevices {
		dev := &allDevices[i]
		if dev.DeviceTypeName != "" {
			filename := path.Base(dev.DeviceTypeName)
			name := strings.TrimSuffix(filename, path.Ext(filename))
			if strings.HasPrefix(name, "Arenti") {
				name = "Arenti " + strings.TrimPrefix(name, "Arenti")
			}
			dev.Model = name
		}

		if dev.SnNum == "" {
			continue
		}

		wg.Add(1)
		go func(d *Device) {
			defer wg.Done()
			battery, wifi, model, ip, err := c.GetDeviceInfo(d.SnNum)
			if err != nil {
				return
			}
			if battery > 0 {
				d.Battery = battery
			}
			if wifi > 0 {
				d.WifiStrength = wifi
			}
			if d.Model == "" && model != "" {
				d.Model = model
			}
			if ip != "" {
				d.IP = ip
			}
		}(dev)
	}
	wg.Wait()

	return allDevices, nil
}

// GetDeviceInfo fetches detailed device telemetry (battery %, wifi %, IP, model) via /ipc_web/device/info
func (c *Client) GetDeviceInfo(snNum string) (battery, wifi int, model, ip string, err error) {
	rawQuery := "snNum=" + url.QueryEscape(snNum)
	respBytes, err := c.doAuthRequest(http.MethodGet, "/ipc_web/device/info", rawQuery, nil)
	if err != nil {
		return 0, 0, "", "", err
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return 0, 0, "", "", err
	}
	if resp.Code != "1001" {
		return 0, 0, "", "", fmt.Errorf("arenti: device info error code %s: %s", resp.Code, resp.Msg)
	}

	if val, ok := resp.Data.Data["204"].(string); ok {
		var p struct {
			Wifi int `json:"wifi"`
			Bt   int `json:"bt"`
		}
		if json.Unmarshal([]byte(val), &p) == nil {
			battery = p.Bt
			wifi = p.Wifi
		}
	}
	if val, ok := resp.Data.Data["63"].(string); ok && val != "" {
		model = val
	}
	if val, ok := resp.Data.Data["126"].(string); ok {
		ip = val
	}

	return battery, wifi, model, ip, nil
}

// GetDevice finds a device by name, serial number (with or without ppsl), or device ID.
func (c *Client) GetDevice(target string) (*Device, error) {
	devices, err := c.GetDevices()
	if err != nil {
		return nil, err
	}

	targetClean := strings.TrimSpace(strings.ToLower(target))
	for _, dev := range devices {
		if strings.ToLower(dev.DeviceName) == targetClean {
			return &dev, nil
		}
		if strings.ToLower(dev.SnNum) == targetClean {
			return &dev, nil
		}
		snWithoutPrefix := strings.TrimPrefix(strings.ToLower(dev.SnNum), "ppsl")
		if snWithoutPrefix == targetClean {
			return &dev, nil
		}
		if fmt.Sprintf("%d", dev.DeviceID) == targetClean {
			return &dev, nil
		}
	}

	return nil, fmt.Errorf("arenti: device not found: %s", target)
}

// GetDeviceStatus checks device connectivity status ("dormancy", "online", "offline").
func (c *Client) GetDeviceStatus(deviceID string) (string, error) {
	rawQuery := fmt.Sprintf("deviceid=%s", deviceID)
	respBytes, err := c.doAuthRequest(http.MethodGet, "/ipc_web/iot/query_device_status", rawQuery, nil)
	if err != nil {
		return "", err
	}

	var statusResp DeviceStatusResponse
	if err := json.Unmarshal(respBytes, &statusResp); err != nil {
		return "", err
	}

	for _, s := range statusResp.Data {
		if s.DeviceID == deviceID {
			return s.Status, nil
		}
	}

	return "unknown", nil
}

// GetSignWss requests a WebSocket signaling ticket for MTS WebRTC connection.
func (c *Client) GetSignWss(callee, deviceCode, expires string) (*SignWssResponse, error) {
	if expires == "" {
		expires = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	rawParams := fmt.Sprintf("expires=%s&method=mts:option&deviceid=%s&devicecode=%s",
		expires, callee, deviceCode)

	respBytes, err := c.doAuthRequest(http.MethodGet, "/ipc_web/iot_sign/wss", rawParams, nil)
	if err != nil {
		return nil, err
	}

	var signResp SignWssResponse
	if err := json.Unmarshal(respBytes, &signResp); err != nil {
		return nil, fmt.Errorf("arenti: decode iot_sign/wss response failed: %w", err)
	}

	if signResp.Code != "1001" {
		return nil, fmt.Errorf("arenti: iot_sign/wss returned code %s: %s", signResp.Code, signResp.Msg)
	}

	return &signResp, nil
}
