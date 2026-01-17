package wyze

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

const (
	FrameSize1080P      = 0
	FrameSize360P       = 1
	FrameSize720P       = 2
	FrameSize2K         = 3
	FrameSizeFloodlight = 4
)

const (
	BitrateMax uint16 = 0xF0
	BitrateSD  uint16 = 0x3C
)

const (
	MediaTypeVideo       = 1
	MediaTypeAudio       = 2
	MediaTypeReturnAudio = 3
	MediaTypeRDT         = 4
)

const (
	KCmdAuth               = 10000
	KCmdChallenge          = 10001
	KCmdChallengeResp      = 10002
	KCmdAuthResult         = 10003
	KCmdControlChannel     = 10010
	KCmdControlChannelResp = 10011
	KCmdSetResolutionDB    = 10052
	KCmdSetResolutionDBRes = 10053
	KCmdSetResolution      = 10056
	KCmdSetResolutionResp  = 10057
)

type Client struct {
	conn *tutk.DTLSConn

	host  string
	uid   string
	enr   string
	mac   string
	model string

	authKey string
	verbose bool

	closed  bool
	closeMu sync.Mutex

	hasAudio    bool
	hasIntercom bool

	audioCodecID    byte
	audioSampleRate uint32
	audioChannels   uint8
}

type AuthResponse struct {
	ConnectionRes string         `json:"connectionRes"`
	CameraInfo    map[string]any `json:"cameraInfo"`
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("wyze: invalid URL: %w", err)
	}

	query := u.Query()

	if query.Get("dtls") != "true" {
		return nil, fmt.Errorf("wyze: only DTLS cameras are supported")
	}

	c := &Client{
		host:    u.Host,
		uid:     query.Get("uid"),
		enr:     query.Get("enr"),
		mac:     query.Get("mac"),
		model:   query.Get("model"),
		verbose: query.Get("verbose") == "true",
	}

	c.authKey = string(tutk.CalculateAuthKey(c.enr, c.mac))

	if c.verbose {
		fmt.Printf("[Wyze] Connecting to %s (UID: %s)\n", c.host, c.uid)
	}

	if err := c.connect(); err != nil {
		c.Close()
		return nil, err
	}

	if err := c.doAVLogin(); err != nil {
		c.Close()
		return nil, err
	}

	if err := c.doKAuth(); err != nil {
		c.Close()
		return nil, err
	}

	if c.verbose {
		fmt.Printf("[Wyze] Connection established\n")
	}

	return c, nil
}

func (c *Client) SupportsAudio() bool {
	return c.hasAudio
}

func (c *Client) SupportsIntercom() bool {
	return c.hasIntercom
}

func (c *Client) SetBackchannelCodec(codecID byte, sampleRate uint32, channels uint8) {
	c.audioCodecID = codecID
	c.audioSampleRate = sampleRate
	c.audioChannels = channels
}

func (c *Client) GetBackchannelCodec() (codecID byte, sampleRate uint32, channels uint8) {
	return c.audioCodecID, c.audioSampleRate, c.audioChannels
}

func (c *Client) SetResolution(quality byte) error {
	var frameSize uint8
	var bitrate uint16

	switch quality {
	case 0: // Auto/HD - use model's best
		frameSize = c.hdFrameSize()
		bitrate = BitrateMax
	case FrameSize360P: // 1 = SD/360P
		frameSize = FrameSize360P
		bitrate = BitrateSD
	case FrameSize720P: // 2 = 720P
		frameSize = FrameSize720P
		bitrate = BitrateMax
	case FrameSize2K: // 3 = 2K
		if c.is2K() {
			frameSize = FrameSize2K
		} else {
			frameSize = c.hdFrameSize()
		}
		bitrate = BitrateMax
	case FrameSizeFloodlight: // 4 = Floodlight
		frameSize = c.hdFrameSize()
		bitrate = BitrateMax
	default:
		frameSize = quality
		bitrate = BitrateMax
	}

	if c.verbose {
		fmt.Printf("[Wyze] SetResolution: quality=%d frameSize=%d bitrate=%d model=%s\n", quality, frameSize, bitrate, c.model)
	}

	// Use K10052 (doorbell format) for certain models
	if c.useDoorbellResolution() {
		k10052 := c.buildK10052(frameSize, bitrate)
		_, err := c.conn.WriteAndWaitIOCtrl(KCmdSetResolutionDB, k10052, KCmdSetResolutionDBRes, 5*time.Second)
		return err
	}

	k10056 := c.buildK10056(frameSize, bitrate)
	_, err := c.conn.WriteAndWaitIOCtrl(KCmdSetResolution, k10056, KCmdSetResolutionResp, 5*time.Second)
	return err
}

func (c *Client) StartVideo() error {
	k10010 := c.buildK10010(MediaTypeVideo, true)
	_, err := c.conn.WriteAndWaitIOCtrl(KCmdControlChannel, k10010, KCmdControlChannelResp, 5*time.Second)
	return err
}

func (c *Client) StartAudio() error {
	k10010 := c.buildK10010(MediaTypeAudio, true)
	_, err := c.conn.WriteAndWaitIOCtrl(KCmdControlChannel, k10010, KCmdControlChannelResp, 5*time.Second)
	return err
}

func (c *Client) StartIntercom() error {
	if c.conn == nil || !c.conn.IsBackchannelReady() {
		return nil
	}

	k10010 := c.buildK10010(MediaTypeReturnAudio, true)
	if _, err := c.conn.WriteAndWaitIOCtrl(KCmdControlChannel, k10010, KCmdControlChannelResp, 5*time.Second); err != nil {
		return err
	}

	return c.conn.AVServStart()
}

func (c *Client) StopIntercom() error {
	if c.conn == nil || !c.conn.IsBackchannelReady() {
		return nil
	}

	k10010 := c.buildK10010(MediaTypeReturnAudio, false)
	c.conn.WriteAndWaitIOCtrl(KCmdControlChannel, k10010, KCmdControlChannelResp, 5*time.Second)

	return c.conn.AVServStop()
}

func (c *Client) ReadPacket() (*tutk.Packet, error) {
	return c.conn.AVRecvFrameData()
}

func (c *Client) WriteAudio(codec byte, payload []byte, timestamp uint32, sampleRate uint32, channels uint8) error {
	if !c.conn.IsBackchannelReady() {
		return fmt.Errorf("speaker channel not connected")
	}

	if c.verbose {
		fmt.Printf("[Wyze] WriteAudio: codec=0x%02x, payload=%d bytes, rate=%d, ch=%d\n", codec, len(payload), sampleRate, channels)
	}

	return c.conn.AVSendAudioData(codec, payload, timestamp, sampleRate, channels)
}

func (c *Client) SetDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetDeadline(t)
	}
	return nil
}

func (c *Client) Protocol() string {
	return "wyze/dtls"
}

func (c *Client) RemoteAddr() net.Addr {
	if c.conn != nil {
		return c.conn.RemoteAddr()
	}
	return nil
}

func (c *Client) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	if c.verbose {
		fmt.Printf("[Wyze] Closing connection\n")
	}

	c.StopIntercom()

	if c.conn != nil {
		c.conn.Close()
	}

	if c.verbose {
		fmt.Printf("[Wyze] Connection closed\n")
	}

	return nil
}

func (c *Client) connect() error {
	host := c.host
	port := 0

	if idx := strings.Index(host, ":"); idx > 0 {
		if p, err := strconv.Atoi(host[idx+1:]); err == nil {
			port = p
		}
		host = host[:idx]
	}

	conn, err := tutk.DialDTLS(host, port, c.uid, c.authKey, c.enr, c.verbose)
	if err != nil {
		return fmt.Errorf("wyze: connect failed: %w", err)
	}

	c.conn = conn
	if c.verbose {
		fmt.Printf("[Wyze] Connected to %s (IOTC + DTLS)\n", conn.RemoteAddr())
	}

	return nil
}

func (c *Client) doAVLogin() error {
	if c.verbose {
		fmt.Printf("[Wyze] Sending AV Login\n")
	}

	if err := c.conn.AVClientStart(5 * time.Second); err != nil {
		return fmt.Errorf("wyze: av login failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] AV Login response received\n")
	}
	return nil
}

func (c *Client) doKAuth() error {
	// Step 1: K10000 -> K10001 (Challenge)
	data, err := c.conn.WriteAndWaitIOCtrl(KCmdAuth, c.buildK10000(), KCmdChallenge, 10*time.Second)
	if err != nil {
		return fmt.Errorf("wyze: K10001 failed: %w", err)
	}

	challenge, status, err := c.parseK10001(data)
	if err != nil {
		return fmt.Errorf("wyze: K10001 parse failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10001 challenge received, status=%d\n", status)
	}

	// Step 2: K10002 -> K10003 (Auth)
	data, err = c.conn.WriteAndWaitIOCtrl(KCmdChallengeResp, c.buildK10002(challenge, status), KCmdAuthResult, 10*time.Second)
	if err != nil {
		return fmt.Errorf("wyze: K10002 failed: %w", err)
	}

	// Parse K10003 response
	authResp, err := c.parseK10003(data)
	if err != nil {
		return fmt.Errorf("wyze: K10003 parse failed: %w", err)
	}

	if c.verbose && authResp != nil {
		if jsonBytes, err := json.MarshalIndent(authResp, "", "  "); err == nil {
			fmt.Printf("[Wyze] K10003 response:\n%s\n", jsonBytes)
		}
	}

	// Extract audio capability from cameraInfo
	if authResp != nil && authResp.CameraInfo != nil {
		if channelResult, ok := authResp.CameraInfo["channelRequestResult"].(map[string]any); ok {
			if audio, ok := channelResult["audio"].(string); ok {
				c.hasAudio = audio == "1"
			} else {
				c.hasAudio = true
			}
		} else {
			c.hasAudio = true
		}
	} else {
		c.hasAudio = true
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10003 auth success\n")
	}

	c.hasIntercom = c.conn.HasTwoWayStreaming()

	if c.verbose {
		fmt.Printf("[Wyze] K-auth complete\n")
	}

	return nil
}

func (c *Client) buildK10000() []byte {
	json := []byte(`{"cameraInfo":{"audioEncoderList":[137,138,140]}}`) // 137=PCMU, 138=PCMA, 140=PCM
	b := make([]byte, 16+len(json))
	copy(b, "HL")                                           // magic
	b[2] = 5                                                // version
	binary.LittleEndian.PutUint16(b[4:], KCmdAuth)          // 10000
	binary.LittleEndian.PutUint16(b[6:], uint16(len(json))) // payload len
	copy(b[16:], json)
	return b
}

func (c *Client) buildK10002(challenge []byte, status byte) []byte {
	resp := generateChallengeResponse(challenge, c.enr, status)
	sessionID := make([]byte, 4)
	rand.Read(sessionID)
	b := make([]byte, 38)
	copy(b, "HL")                                           // magic
	b[2] = 5                                                // version
	binary.LittleEndian.PutUint16(b[4:], KCmdChallengeResp) // 10002
	b[6] = 22                                               // payload len
	copy(b[16:], resp[:16])                                 // challenge response
	copy(b[32:], sessionID)                                 // random session ID
	b[36] = 1                                               // video enabled/disabled
	b[37] = 1                                               // audio enabled/disabled
	return b
}

func (c *Client) buildK10010(mediaType byte, enabled bool) []byte {
	b := make([]byte, 18)
	copy(b, "HL")                                            // magic
	b[2] = 5                                                 // version
	binary.LittleEndian.PutUint16(b[4:], KCmdControlChannel) // 10010
	binary.LittleEndian.PutUint16(b[6:], 2)                  // payload len
	b[16] = mediaType                                        // 1=video, 2=audio, 3=return audio
	b[17] = 1                                                // 1=enable, 2=disable
	if !enabled {
		b[17] = 2
	}
	return b
}

func (c *Client) buildK10052(frameSize uint8, bitrate uint16) []byte {
	b := make([]byte, 22)
	copy(b, "HL")                                             // magic
	b[2] = 5                                                  // version
	binary.LittleEndian.PutUint16(b[4:], KCmdSetResolutionDB) // 10052
	binary.LittleEndian.PutUint16(b[6:], 6)                   // payload len
	binary.LittleEndian.PutUint16(b[16:], bitrate)            // bitrate (2 bytes)
	b[18] = frameSize + 1                                     // frame size (1 byte)
	// b[19] = fps, b[20:22] = zeros
	return b
}

func (c *Client) buildK10056(frameSize uint8, bitrate uint16) []byte {
	b := make([]byte, 21)
	copy(b, "HL")                                           // magic
	b[2] = 5                                                // version
	binary.LittleEndian.PutUint16(b[4:], KCmdSetResolution) // 10056
	binary.LittleEndian.PutUint16(b[6:], 5)                 // payload len
	b[16] = frameSize + 1                                   // frame size
	binary.LittleEndian.PutUint16(b[17:], bitrate)          // bitrate
	// b[19:21] = FPS (0 = auto)
	return b
}

func (c *Client) parseK10001(data []byte) (challenge []byte, status byte, err error) {
	if c.verbose {
		fmt.Printf("[Wyze] parseK10001: received %d bytes\n", len(data))
	}

	if len(data) < 33 {
		return nil, 0, fmt.Errorf("data too short: %d bytes", len(data))
	}

	if data[0] != 'H' || data[1] != 'L' {
		return nil, 0, fmt.Errorf("invalid HL magic: %x %x", data[0], data[1])
	}

	cmdID := binary.LittleEndian.Uint16(data[4:])
	if cmdID != KCmdChallenge {
		return nil, 0, fmt.Errorf("expected cmdID 10001, got %d", cmdID)
	}

	status = data[16]
	challenge = make([]byte, 16)
	copy(challenge, data[17:33])

	return challenge, status, nil
}

func (c *Client) parseK10003(data []byte) (*AuthResponse, error) {
	if c.verbose {
		fmt.Printf("[Wyze] parseK10003: received %d bytes\n", len(data))
	}

	if len(data) < 16 {
		return &AuthResponse{}, nil
	}

	if data[0] != 'H' || data[1] != 'L' {
		return &AuthResponse{}, nil
	}

	cmdID := binary.LittleEndian.Uint16(data[4:])
	textLen := binary.LittleEndian.Uint16(data[6:])

	if cmdID != KCmdAuthResult {
		return &AuthResponse{}, nil
	}

	if len(data) > 16 && textLen > 0 {
		jsonData := data[16:]
		for i := range jsonData {
			if jsonData[i] == '{' {
				var resp AuthResponse
				if err := json.Unmarshal(jsonData[i:], &resp); err == nil {
					if c.verbose {
						fmt.Printf("[Wyze] parseK10003: parsed JSON\n")
					}
					return &resp, nil
				}
				break
			}
		}
	}

	return &AuthResponse{}, nil
}

func (c *Client) useDoorbellResolution() bool {
	switch c.model {
	case "WYZEDB3", "WVOD1", "HL_WCO2", "WYZEC1":
		return true
	}
	return false
}

func (c *Client) hdFrameSize() uint8 {
	if c.isFloodlight() {
		return FrameSizeFloodlight
	}
	if c.is2K() {
		return FrameSize2K
	}
	return FrameSize1080P
}

func (c *Client) is2K() bool {
	switch c.model {
	case "HL_CAM3P", "HL_PANP", "HL_CAM4", "HL_DB2", "HL_CFL2":
		return true
	}
	return false
}

func (c *Client) isFloodlight() bool {
	return c.model == "HL_CFL2"
}

const (
	statusDefault byte = 1
	statusENR16   byte = 3
	statusENR32   byte = 6
)

func generateChallengeResponse(challengeBytes []byte, enr string, status byte) []byte {
	var secretKey []byte

	switch status {
	case statusDefault:
		secretKey = []byte("FFFFFFFFFFFFFFFF")
	case statusENR16:
		if len(enr) >= 16 {
			secretKey = []byte(enr[:16])
		} else {
			secretKey = make([]byte, 16)
			copy(secretKey, enr)
		}
	case statusENR32:
		if len(enr) >= 16 {
			firstKey := []byte(enr[:16])
			challengeBytes = tutk.XXTEADecryptVar(challengeBytes, firstKey)
		}
		if len(enr) >= 32 {
			secretKey = []byte(enr[16:32])
		} else if len(enr) > 16 {
			secretKey = make([]byte, 16)
			copy(secretKey, []byte(enr[16:]))
		} else {
			secretKey = []byte("FFFFFFFFFFFFFFFF")
		}
	default:
		secretKey = []byte("FFFFFFFFFFFFFFFF")
	}

	return tutk.XXTEADecryptVar(challengeBytes, secretKey)
}
