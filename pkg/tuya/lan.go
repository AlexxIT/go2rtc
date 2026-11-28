package tuya

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

var (
	v34Prefix = []byte{0x00, 0x00, 0x55, 0xAA}
	v34Suffix = []byte{0x00, 0x00, 0xAA, 0x55}
)

type TuyaLanConfig struct {
	CameraIP        string `json:"camera_ip"`
	Port            int    `json:"port"`
	DeviceID        string `json:"device_id"`
	SenderID        string `json:"sender_id"`
	LocalKey        string `json:"local_key"`
	HGWVersion      string `json:"hgw_version"`
	WebRTCAuth      string `json:"webrtc_auth"`
	MotoID          string `json:"moto_id"`
	Skill           string `json:"skill"`
	ProtocolVersion string `json:"protocol_version"`
	StunURL         string `json:"stun_url,omitempty"`
}

type TuyaLanAPIClient struct {
	TuyaClient
	lan *TuyaLanSignal
}

func NewTuyaLanApiClient(u *url.URL) (*TuyaLanAPIClient, error) {
	var config TuyaLanConfig

	switch {
	case u.Host != "":
		query := u.Query()
		config = TuyaLanConfig{
			CameraIP:        u.Hostname(),
			Port:            core.Atoi(query.Get("port")),
			DeviceID:        query.Get("device_id"),
			SenderID:        query.Get("sender_id"),
			LocalKey:        query.Get("local_key"),
			HGWVersion:      query.Get("hgw_version"),
			WebRTCAuth:      query.Get("webrtc_auth"),
			MotoID:          query.Get("moto_id"),
			Skill:           query.Get("skill"),
			ProtocolVersion: query.Get("protocol_version"),
			StunURL:         query.Get("stun"),
		}
	case u.Path != "":
		data, err := os.ReadFile(u.Path)
		if err != nil {
			return nil, fmt.Errorf("tuya lan: read config: %w", err)
		}
		if err = json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("tuya lan: decode config: %w", err)
		}
	default:
		return nil, errors.New("tuya lan: camera address or config path is required")
	}

	if config.HGWVersion == "" {
		config.HGWVersion = "3.4"
	}
	if config.CameraIP == "" || config.DeviceID == "" {
		return nil, errors.New("tuya lan: camera address and device_id are required")
	}
	if len(config.LocalKey) != 16 {
		return nil, errors.New("tuya lan: local_key must be 16 bytes")
	}
	if config.HGWVersion != "3.4" {
		return nil, fmt.Errorf("tuya lan: unsupported hgw_version %q", config.HGWVersion)
	}
	if config.Port == 0 {
		config.Port = 6668
	}
	if config.SenderID == "" {
		config.SenderID = "go2rtc-" + core.RandString(11, 62)
	}

	skill := &Skill{}
	if config.Skill != "" {
		if err := json.Unmarshal([]byte(config.Skill), skill); err != nil {
			return nil, fmt.Errorf("tuya lan: decode skill: %w", err)
		}
	}

	lan := NewTuyaLanSignal(&config)
	client := &TuyaLanAPIClient{
		TuyaClient: TuyaClient{
			deviceId: config.DeviceID,
			localKey: config.LocalKey,
			skill:    skill,
			signal:   lan,
		},
		lan: lan,
	}
	return client, nil
}

// GetAudioCodecs returns the audio codecs offered over the LAN channel. Tuya reports
// the camera's G.711 audio as codecType 101, which the cloud path maps to PCML, but
// the LAN answer negotiates G.711/8000 sendrecv. Advertise both G.711 flavours so a
// browser microphone, which only offers G.711/Opus, can match the backchannel track.
func (c *TuyaLanAPIClient) GetAudioCodecs() []*core.Codec {
	if c.skill == nil || len(c.skill.Audios) == 0 {
		return []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1},
			{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1},
		}
	}

	codecs := make([]*core.Codec, 0, len(c.skill.Audios)*2)
	for _, audio := range c.skill.Audios {
		codec := &core.Codec{
			Name:      getAudioCodecName(&audio),
			ClockRate: uint32(audio.SampleRate),
			Channels:  uint8(audio.Channels),
		}
		if codec.Name == core.CodecPCML {
			codec.Name = core.CodecPCMA
		}
		codecs = append(codecs, codec)

		switch codec.Name {
		case core.CodecPCMA:
			codecs = append(codecs, &core.Codec{Name: core.CodecPCMU,
				ClockRate: codec.ClockRate, Channels: codec.Channels})
		case core.CodecPCMU:
			codecs = append(codecs, &core.Codec{Name: core.CodecPCMA,
				ClockRate: codec.ClockRate, Channels: codec.Channels})
		}
	}
	return codecs
}

func (c *TuyaLanAPIClient) Init() error {
	return c.lan.Start()
}

func (c *TuyaLanAPIClient) GetStreamUrl(rawURL string) (string, error) {
	return rawURL, nil
}

// LAN signaling on this camera carries video over Tuya's private KCP transport.
// Keep the standards-based bidirectional audio m-line and reject the unusable
// video section so Pion can establish the microphone channel.
func (c *TuyaLanAPIClient) IsHEVC(int) bool {
	return false
}

func normalizeTuyaLanAnswer(sdp string) string {
	if !strings.Contains(sdp, "m=application 9 tuya") {
		return sdp
	}
	sdp = strings.Replace(sdp, "a=group:BUNDLE 0 1 2", "a=group:BUNDLE 0 1", 1)
	lines := strings.Split(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "m=video ") {
			break
		}
		if line != "" {
			kept = append(kept, line)
		}
	}
	kept = append(kept,
		"m=video 0 UDP/TLS/RTP/SAVPF 102",
		"c=IN IP4 0.0.0.0",
		"a=mid:1",
		"a=inactive",
		"a=rtpmap:102 H264/90000",
	)
	return strings.Join(kept, "\r\n") + "\r\n"
}

type TuyaLanSignal struct {
	config *TuyaLanConfig
	conn   net.Conn

	key        []byte
	sessionKey []byte
	sessionID  string
	sequence   uint32

	writeMu   sync.Mutex
	handlerMu sync.RWMutex
	closed    atomic.Bool

	handleAnswer     func(AnswerFrame)
	handleCandidate  func(CandidateFrame)
	handleDisconnect func()
	handleError      func(error)
}

func NewTuyaLanSignal(config *TuyaLanConfig) *TuyaLanSignal {
	return &TuyaLanSignal{
		config:    config,
		key:       []byte(config.LocalKey),
		sessionID: core.RandString(6, 62),
	}
}

func (c *TuyaLanSignal) SetHandlers(answer func(AnswerFrame), candidate func(CandidateFrame), disconnect func(), handleError func(error)) {
	c.handlerMu.Lock()
	c.handleAnswer = answer
	c.handleCandidate = candidate
	c.handleDisconnect = disconnect
	c.handleError = handleError
	c.handlerMu.Unlock()
}

func (c *TuyaLanSignal) Start() error {
	address := net.JoinHostPort(c.config.CameraIP, fmt.Sprintf("%d", c.config.Port))
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("tuya lan: connect: %w", err)
	}
	c.conn = conn
	_ = c.conn.SetDeadline(time.Now().Add(8 * time.Second))

	clientNonce := make([]byte, 16)
	if _, err = rand.Read(clientNonce); err != nil {
		c.conn.Close()
		return err
	}
	if err = c.writeFrame(3, clientNonce, c.key); err != nil {
		c.conn.Close()
		return fmt.Errorf("tuya lan: session start: %w", err)
	}

	raw, err := readV34Frame(c.conn)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("tuya lan: session response: %w", err)
	}
	command, status, payload, err := decodeV34Frame(raw, c.key, true)
	if err != nil || command != 4 || status != 0 || len(payload) != 48 {
		c.conn.Close()
		return fmt.Errorf("tuya lan: invalid session response command=%d status=%d size=%d: %w", command, status, len(payload), err)
	}

	remoteNonce := payload[:16]
	expected := hmac.New(sha256.New, c.key)
	expected.Write(clientNonce)
	if !hmac.Equal(expected.Sum(nil), payload[16:]) {
		c.conn.Close()
		return errors.New("tuya lan: invalid session nonce signature")
	}

	finish := hmac.New(sha256.New, c.key)
	finish.Write(remoteNonce)
	if err = c.writeFrame(5, finish.Sum(nil), c.key); err != nil {
		c.conn.Close()
		return fmt.Errorf("tuya lan: session finish: %w", err)
	}

	mixed := make([]byte, 16)
	for i := range mixed {
		mixed[i] = clientNonce[i] ^ remoteNonce[i]
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		c.conn.Close()
		return err
	}
	c.sessionKey = make([]byte, 16)
	block.Encrypt(c.sessionKey, mixed)
	_ = c.conn.SetDeadline(time.Time{})
	go c.readLoop()
	return nil
}

func (c *TuyaLanSignal) Stop() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	if c.conn != nil {
		_ = c.sendSignal("disconnect", DisconnectFrame{Mode: "webrtc"})
		_ = c.conn.Close()
	}
}

func (c *TuyaLanSignal) SendOffer(sdp, streamResolution string, streamType int, isHEVC bool) error {
	mqttStreamType := streamType
	switch streamType {
	case 2:
		mqttStreamType = 0
	case 4:
		mqttStreamType = 1
	}
	frame := OfferFrame{
		Mode:              "webrtc",
		Sdp:               sdp,
		StreamType:        mqttStreamType,
		Auth:              c.config.WebRTCAuth,
		DatachannelEnable: isHEVC,
	}
	if c.config.StunURL != "" {
		// The camera uses this as an ICE server for its own candidates, it is only
		// needed when the camera can't reach this host directly (different subnet).
		frame.Token = []ICEServer{{Urls: c.config.StunURL}}
	}
	return c.sendSignal("offer", frame)
}

func (c *TuyaLanSignal) SendCandidate(candidate string) error {
	return c.sendSignal("candidate", CandidateFrame{Mode: "webrtc", Candidate: candidate})
}

func (c *TuyaLanSignal) SendResolution(resolution int) error {
	return c.sendSignal("resolution", ResolutionFrame{Mode: "webrtc", Value: resolution})
}

func (c *TuyaLanSignal) SendSpeaker(speaker int) error {
	return c.sendSignal("speaker", SpeakerFrame{Mode: "webrtc", Value: speaker})
}

func (c *TuyaLanSignal) SendDisconnect() error {
	return c.sendSignal("disconnect", DisconnectFrame{Mode: "webrtc"})
}

func (c *TuyaLanSignal) sendSignal(messageType string, message any) error {
	if c.closed.Load() && messageType != "disconnect" {
		return errors.New("tuya lan: client is closed")
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	frame := MqttFrame{
		Header: MqttFrameHeader{
			Type:      messageType,
			From:      c.config.SenderID,
			To:        c.config.DeviceID,
			SessionID: c.sessionID,
			MotoID:    c.config.MotoID,
			Path:      "lan",
		},
		Message: body,
	}
	payload, err := json.Marshal(&frame)
	if err != nil {
		return err
	}
	return c.writeFrame(32, payload, c.sessionKey)
}

func (c *TuyaLanSignal) writeFrame(command uint32, payload, key []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.sequence++
	raw, err := encodeV34Frame(c.sequence, command, payload, key)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(raw)
	return err
}

func (c *TuyaLanSignal) readLoop() {
	for {
		raw, err := readV34Frame(c.conn)
		if err != nil {
			if !c.closed.Load() {
				c.onError(fmt.Errorf("tuya lan: read: %w", err))
			}
			return
		}
		command, status, payload, err := decodeV34Frame(raw, c.sessionKey, true)
		if err != nil {
			c.onError(fmt.Errorf("tuya lan: decode: %w", err))
			return
		}
		if command != 32 || status != 0 || len(payload) == 0 {
			continue
		}

		var frame MqttFrame
		if err = json.Unmarshal(payload, &frame); err != nil {
			c.onError(fmt.Errorf("tuya lan: signal json: %w", err))
			continue
		}
		if frame.Header.SessionID != c.sessionID {
			continue
		}

		switch frame.Header.Type {
		case "answer":
			var answer AnswerFrame
			if err = json.Unmarshal(frame.Message, &answer); err == nil {
				c.handlerMu.RLock()
				handler := c.handleAnswer
				c.handlerMu.RUnlock()
				if handler != nil {
					handler(answer)
				}
			}
		case "candidate":
			var candidate CandidateFrame
			if err = json.Unmarshal(frame.Message, &candidate); err == nil {
				candidate.Candidate = strings.TrimSuffix(strings.TrimPrefix(candidate.Candidate, "a="), "\r\n")
				c.handlerMu.RLock()
				handler := c.handleCandidate
				c.handlerMu.RUnlock()
				if handler != nil {
					handler(candidate)
				}
			}
		case "disconnect":
			c.handlerMu.RLock()
			handler := c.handleDisconnect
			c.handlerMu.RUnlock()
			if handler != nil {
				handler()
			}
		}
	}
}

func (c *TuyaLanSignal) onError(err error) {
	c.handlerMu.RLock()
	handler := c.handleError
	c.handlerMu.RUnlock()
	if handler != nil {
		handler(err)
	}
}

func encodeV34Frame(sequence, command uint32, payload, key []byte) ([]byte, error) {
	ciphertext, err := encryptECBPKCS7(payload, key)
	if err != nil {
		return nil, err
	}
	length := uint32(len(ciphertext) + sha256.Size + len(v34Suffix))
	raw := make([]byte, 16+len(ciphertext)+sha256.Size+len(v34Suffix))
	copy(raw, v34Prefix)
	binary.BigEndian.PutUint32(raw[4:8], sequence)
	binary.BigEndian.PutUint32(raw[8:12], command)
	binary.BigEndian.PutUint32(raw[12:16], length)
	copy(raw[16:], ciphertext)
	macStart := 16 + len(ciphertext)
	mac := hmac.New(sha256.New, key)
	mac.Write(raw[:macStart])
	copy(raw[macStart:], mac.Sum(nil))
	copy(raw[macStart+sha256.Size:], v34Suffix)
	return raw, nil
}

func readV34Frame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 16)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	if !hmac.Equal(header[:4], v34Prefix) {
		return nil, errors.New("invalid frame prefix")
	}
	length := binary.BigEndian.Uint32(header[12:16])
	if length < sha256.Size+4 || length > 1<<20 {
		return nil, fmt.Errorf("invalid frame length %d", length)
	}
	raw := make([]byte, 16+int(length))
	copy(raw, header)
	if _, err := io.ReadFull(reader, raw[16:]); err != nil {
		return nil, err
	}
	return raw, nil
}

func decodeV34Frame(raw, key []byte, hasStatus bool) (command uint32, status uint32, payload []byte, err error) {
	if len(raw) < 16+sha256.Size+4 || !hmac.Equal(raw[:4], v34Prefix) || !hmac.Equal(raw[len(raw)-4:], v34Suffix) {
		return 0, 0, nil, errors.New("invalid frame")
	}
	macStart := len(raw) - sha256.Size - 4
	mac := hmac.New(sha256.New, key)
	mac.Write(raw[:macStart])
	if !hmac.Equal(mac.Sum(nil), raw[macStart:macStart+sha256.Size]) {
		return 0, 0, nil, errors.New("invalid frame hmac")
	}
	command = binary.BigEndian.Uint32(raw[8:12])
	body := raw[16:macStart]
	if hasStatus {
		if len(body) < 4 {
			return command, 0, nil, errors.New("missing response status")
		}
		status = binary.BigEndian.Uint32(body[:4])
		body = body[4:]
	}
	if len(body) == 0 {
		return command, status, nil, nil
	}
	payload, err = decryptECBPKCS7(body, key)
	return command, status, payload, err
}

func encryptECBPKCS7(payload, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padding := block.BlockSize() - len(payload)%block.BlockSize()
	padded := make([]byte, len(payload)+padding)
	copy(padded, payload)
	for i := len(payload); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(out[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}
	return out, nil
}

func decryptECBPKCS7(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("invalid encrypted payload length")
	}
	out := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += block.BlockSize() {
		block.Decrypt(out[i:i+block.BlockSize()], ciphertext[i:i+block.BlockSize()])
	}
	padding := int(out[len(out)-1])
	if padding == 0 || padding > block.BlockSize() || padding > len(out) {
		return nil, errors.New("invalid payload padding")
	}
	for _, value := range out[len(out)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid payload padding")
		}
	}
	return out[:len(out)-padding], nil
}

var _ TuyaSignalClient = (*TuyaLanSignal)(nil)
