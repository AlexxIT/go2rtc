package wyze

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/wyze/crypto"
	"github.com/AlexxIT/go2rtc/pkg/wyze/tutk"
)

type Client struct {
	conn *tutk.Conn

	host string
	uid  string
	enr  string
	mac  string

	authKey string
	verbose bool

	closed  bool
	closeMu sync.Mutex

	hasAudio    bool
	hasIntercom bool

	audioCodecID    uint16
	audioSampleRate uint32
	audioChannels   uint8
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
		verbose: query.Get("verbose") == "true",
	}

	c.authKey = string(crypto.CalculateAuthKey(c.enr, c.mac))

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

func (c *Client) SetBackchannelCodec(codecID uint16, sampleRate uint32, channels uint8) {
	c.audioCodecID = codecID
	c.audioSampleRate = sampleRate
	c.audioChannels = channels
}

func (c *Client) GetBackchannelCodec() (codecID uint16, sampleRate uint32, channels uint8) {
	return c.audioCodecID, c.audioSampleRate, c.audioChannels
}

func (c *Client) SetResolution(sd bool) error {
	var frameSize uint8
	var bitrate uint16

	if sd {
		frameSize = tutk.FrameSize360P
		bitrate = tutk.BitrateSD
	} else {
		frameSize = tutk.FrameSize2K
		bitrate = tutk.BitrateMax
	}

	if c.verbose {
		fmt.Printf("[Wyze] SetResolution: sd=%v frameSize=%d bitrate=%d\n", sd, frameSize, bitrate)
	}

	k10056 := c.buildK10056(frameSize, bitrate)
	if err := c.conn.SendIOCtrl(tutk.KCmdSetResolution, k10056); err != nil {
		return fmt.Errorf("wyze: K10056 send failed: %w", err)
	}

	// Wait for K10057 response
	cmdID, data, err := c.conn.RecvIOCtrl(1 * time.Second)
	if err != nil {
		return err
	}

	if c.verbose {
		fmt.Printf("[Wyze] SetResolution response: K%d (%d bytes)\n", cmdID, len(data))
	}

	if cmdID == tutk.KCmdSetResolutionResp && len(data) >= 17 {
		result := data[16]
		if c.verbose {
			fmt.Printf("[Wyze] K10057 result: %d\n", result)
		}
	}

	return nil
}

func (c *Client) StartVideo() error {
	k10010 := c.buildK10010(tutk.MediaTypeVideo, true)
	if c.verbose {
		fmt.Printf("[Wyze] TX K10010 video (%d bytes): % x\n", len(k10010), k10010)
	}

	if err := c.conn.SendIOCtrl(tutk.KCmdControlChannel, k10010); err != nil {
		return fmt.Errorf("K10010 video send failed: %w", err)
	}

	// Wait for K10011 response
	cmdID, data, err := c.conn.RecvIOCtrl(5 * time.Second)
	if err != nil {
		return fmt.Errorf("K10011 video recv failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10011 video response: cmdID=%d, len=%d\n", cmdID, len(data))
		if len(data) >= 18 {
			fmt.Printf("[Wyze] K10011 video: media=%d status=%d\n", data[16], data[17])
		}
	}

	return nil
}

func (c *Client) StartAudio() error {
	k10010 := c.buildK10010(tutk.MediaTypeAudio, true)
	if c.verbose {
		fmt.Printf("[Wyze] TX K10010 audio (%d bytes): % x\n", len(k10010), k10010)
	}

	if err := c.conn.SendIOCtrl(tutk.KCmdControlChannel, k10010); err != nil {
		return fmt.Errorf("K10010 audio send failed: %w", err)
	}

	// Wait for K10011 response
	cmdID, data, err := c.conn.RecvIOCtrl(5 * time.Second)
	if err != nil {
		return fmt.Errorf("K10011 audio recv failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10011 audio response: cmdID=%d, len=%d\n", cmdID, len(data))
		if len(data) >= 18 {
			fmt.Printf("[Wyze] K10011 audio: media=%d status=%d\n", data[16], data[17])
		}
	}

	return nil
}

func (c *Client) StartIntercom() error {
	if c.conn.IsBackchannelReady() {
		return nil // Already enabled
	}

	if c.verbose {
		fmt.Printf("[Wyze] Sending K10010 (enable return audio)\n")
	}

	k10010 := c.buildK10010(tutk.MediaTypeReturnAudio, true)
	if err := c.conn.SendIOCtrl(tutk.KCmdControlChannel, k10010); err != nil {
		return fmt.Errorf("K10010 send failed: %w", err)
	}

	// Wait for K10011 response
	cmdID, data, err := c.conn.RecvIOCtrl(5 * time.Second)
	if err != nil {
		return fmt.Errorf("K10011 recv failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10011 response: cmdID=%d, len=%d\n", cmdID, len(data))
	}

	// Perform DTLS server handshake on backchannel (camera connects to us)
	if c.verbose {
		fmt.Printf("[Wyze] Starting speaker channel DTLS handshake\n")
	}

	if err := c.conn.AVServStart(); err != nil {
		return fmt.Errorf("speaker channel handshake failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] Backchannel ready\n")
	}

	return nil
}

func (c *Client) ReadPacket() (*tutk.Packet, error) {
	return c.conn.AVRecvFrameData()
}

func (c *Client) WriteAudio(codec uint16, payload []byte, timestamp uint32, sampleRate uint32, channels uint8) error {
	if !c.conn.IsBackchannelReady() {
		return fmt.Errorf("speaker channel not connected")
	}

	if c.verbose {
		fmt.Printf("[Wyze] WriteAudio: codec=0x%04x, payload=%d bytes, rate=%d, ch=%d\n", codec, len(payload), sampleRate, channels)
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

	if c.conn != nil {
		c.conn.Close()
	}

	return nil
}

func (c *Client) connect() error {
	host := c.host
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}

	conn, err := tutk.Dial(host, c.uid, c.authKey, c.enr, c.mac, c.verbose)
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
		return fmt.Errorf("wyze: AV login failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] AV Login response received\n")
	}
	return nil
}

func (c *Client) doKAuth() error {
	if c.verbose {
		fmt.Printf("[Wyze] Starting K-command authentication\n")
	}

	// Step 1: Send K10000
	k10000 := c.buildK10000()
	if err := c.conn.SendIOCtrl(tutk.KCmdAuth, k10000); err != nil {
		return fmt.Errorf("wyze: K10000 send failed: %w", err)
	}

	// Step 2: Wait for K10001
	cmdID, data, err := c.conn.RecvIOCtrl(10 * time.Second)
	if err != nil {
		return fmt.Errorf("wyze: K10001 recv failed: %w", err)
	}
	if cmdID != tutk.KCmdChallenge {
		return fmt.Errorf("wyze: expected K10001, got K%d", cmdID)
	}

	challenge, status, err := c.parseK10001(data)
	if err != nil {
		return fmt.Errorf("wyze: K10001 parse failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[Wyze] K10001 received, status=%d\n", status)
	}

	// Step 3: Send K10002
	k10002 := c.buildK10002(challenge, status)
	if err := c.conn.SendIOCtrl(tutk.KCmdChallengeResp, k10002); err != nil {
		return fmt.Errorf("wyze: K10002 send failed: %w", err)
	}

	// Step 4: Wait for K10003
	cmdID, data, err = c.conn.RecvIOCtrl(10 * time.Second)
	if err != nil {
		return fmt.Errorf("wyze: K10003 recv failed: %w", err)
	}
	if cmdID != tutk.KCmdAuthResult {
		return fmt.Errorf("wyze: expected K10003, got K%d", cmdID)
	}

	authResp, err := c.parseK10003(data)
	if err != nil {
		return fmt.Errorf("wyze: K10003 parse failed: %w", err)
	}

	// Parse capabilities
	if authResp != nil && authResp.CameraInfo != nil {
		if c.verbose {
			fmt.Printf("[Wyze] CameraInfo authResp: ")
			b, _ := json.Marshal(authResp)
			fmt.Printf("%s\n", b)
		}

		// Audio receiving support
		if audio, ok := authResp.CameraInfo["audio"].(bool); ok {
			c.hasAudio = audio
		} else {
			c.hasAudio = true // Default to true
		}
	} else {
		c.hasAudio = true
	}

	if avResp := c.conn.GetAVLoginResponse(); avResp != nil {
		c.hasIntercom = avResp.TwoWayStreaming == 1
		if c.verbose {
			fmt.Printf("[Wyze] two_way_streaming=%d (from AV Login Response)\n", avResp.TwoWayStreaming)
		}
	}

	if c.verbose {
		fmt.Printf("[Wyze] K-auth complete\n")
	}

	return nil
}

func (c *Client) buildK10000() []byte {
	buf := make([]byte, 16)
	buf[0] = 'H'
	buf[1] = 'L'
	buf[2] = 5
	binary.LittleEndian.PutUint16(buf[4:], tutk.KCmdAuth)
	return buf
}

func (c *Client) buildK10002(challenge []byte, status byte) []byte {
	response := crypto.GenerateChallengeResponse(challenge, c.enr, status)

	buf := make([]byte, 38)
	buf[0] = 'H'
	buf[1] = 'L'
	buf[2] = 5
	binary.LittleEndian.PutUint16(buf[4:], tutk.KCmdChallengeResp)
	buf[6] = 22 // Payload length

	if len(response) >= 16 {
		copy(buf[16:], response[:16])
	}

	if len(c.uid) >= 4 {
		copy(buf[32:], c.uid[:4])
	}

	buf[36] = 1 // Video flag (0 = disabled, 1 = enabled > will start video stream immediately)
	buf[37] = 1 // Audio flag (0 = disabled, 1 = enabled > will start audio stream immediately)

	return buf
}

func (c *Client) buildK10010(mediaType byte, enabled bool) []byte {
	// SDK format: 18 bytes total
	// Header: 16 bytes, Payload: 2 bytes (media_type + enabled)
	// TX K10010: 48 4c 05 00 1a 27 02 00 00 00 00 00 00 00 00 00 01 01
	buf := make([]byte, 18)
	buf[0] = 'H'
	buf[1] = 'L'
	buf[2] = 5                                                      // Version
	binary.LittleEndian.PutUint16(buf[4:], tutk.KCmdControlChannel) // 0x271a = 10010
	binary.LittleEndian.PutUint16(buf[6:], 2)                       // Payload length = 2
	buf[16] = mediaType                                             // 1=Video, 2=Audio, 3=ReturnAudio
	if enabled {
		buf[17] = 1
	} else {
		buf[17] = 2
	}
	return buf
}

func (c *Client) buildK10056(frameSize uint8, bitrate uint16) []byte {
	// SDK format: 21 bytes total
	// Header: 16 bytes, Payload: 5 bytes
	// TX K10056: 48 4c 05 00 48 27 05 00 00 00 00 00 00 00 00 00 04 f0 00 00 00
	buf := make([]byte, 21)
	buf[0] = 'H'
	buf[1] = 'L'
	buf[2] = 5                                                     // Version
	binary.LittleEndian.PutUint16(buf[4:], tutk.KCmdSetResolution) // 0x2748 = 10056
	binary.LittleEndian.PutUint16(buf[6:], 5)                      // Payload length = 5
	buf[16] = frameSize + 1                                        // 4 = HD
	binary.LittleEndian.PutUint16(buf[17:], bitrate)               // 0x00f0 = 240
	// buf[19], buf[20] = FPS (0 = auto)
	return buf
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
	if cmdID != tutk.KCmdChallenge {
		return nil, 0, fmt.Errorf("expected cmdID 10001, got %d", cmdID)
	}

	status = data[16]
	challenge = make([]byte, 16)
	copy(challenge, data[17:33])

	return challenge, status, nil
}

func (c *Client) parseK10003(data []byte) (*tutk.AuthResponse, error) {
	if c.verbose {
		fmt.Printf("[Wyze] parseK10003: received %d bytes\n", len(data))
	}

	if len(data) < 16 {
		return &tutk.AuthResponse{}, nil
	}

	if data[0] != 'H' || data[1] != 'L' {
		return &tutk.AuthResponse{}, nil
	}

	cmdID := binary.LittleEndian.Uint16(data[4:])
	textLen := binary.LittleEndian.Uint16(data[6:])

	if cmdID != tutk.KCmdAuthResult {
		return &tutk.AuthResponse{}, nil
	}

	if len(data) > 16 && textLen > 0 {
		jsonData := data[16:]
		for i := range jsonData {
			if jsonData[i] == '{' {
				var resp tutk.AuthResponse
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

	return &tutk.AuthResponse{}, nil
}
