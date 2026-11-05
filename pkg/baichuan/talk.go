package baichuan

import (
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"sync"
)

// UnsupportedTalkError reports that the camera does not expose a usable talkback profile.
type UnsupportedTalkError struct {
	Reason string
}

func (e *UnsupportedTalkError) Error() string {
	return fmt.Sprintf("camera does not support talkback: %s", e.Reason)
}

// TalkAudioConfig describes the audio format accepted by the camera for talkback.
type TalkAudioConfig struct {
	Priority        *uint32 `xml:"priority,omitempty"`
	AudioType       string  `xml:"audioType"`
	SampleRate      uint16  `xml:"sampleRate"`
	SamplePrecision uint16  `xml:"samplePrecision"`
	LengthPerEncode uint16  `xml:"lengthPerEncoder"`
	SoundTrack      string  `xml:"soundTrack"`
}

// TalkConfig describes the requested talkback session parameters.
type TalkConfig struct {
	Version         string          `xml:"version,attr"`
	ChannelID       uint8           `xml:"channelId"`
	Duplex          string          `xml:"duplex"`
	AudioStreamMode string          `xml:"audioStreamMode"`
	AudioConfig     TalkAudioConfig `xml:"audioConfig"`
}

// TalkAbility is the talk capability block returned by the camera.
type TalkAbility struct {
	Version             string                  `xml:"version,attr"`
	DuplexList          []talkDuplexOption      `xml:"duplexList"`
	AudioStreamModeList []talkAudioStreamMode   `xml:"audioStreamModeList"`
	AudioConfigList     []talkAudioConfigOption `xml:"audioConfigList"`
}

type talkDuplexOption struct {
	Duplex string `xml:"duplex"`
}

type talkAudioStreamMode struct {
	AudioStreamMode string `xml:"audioStreamMode"`
}

type talkAudioConfigOption struct {
	AudioConfig TalkAudioConfig `xml:"audioConfig"`
}

type talkAbilityBody struct {
	XMLName     xml.Name     `xml:"body"`
	TalkAbility *TalkAbility `xml:"TalkAbility"`
}

type talkConfigBody struct {
	XMLName    xml.Name    `xml:"body"`
	TalkConfig *TalkConfig `xml:"TalkConfig"`
}

type talkExtension struct {
	XMLName    xml.Name `xml:"Extension"`
	Version    string   `xml:"version,attr"`
	ChannelID  uint8    `xml:"channelId"`
	BinaryData *int     `xml:"binaryData,omitempty"`
}

// TalkSession streams ADPCM blocks to a camera speaker.
type TalkSession struct {
	client          *Client
	channel         uint8
	msgNum          uint16
	binaryExtension []byte
	sampleRate      int
	samplesPerBlock int
	bytesPerBlock   int
	mu              sync.Mutex
	closed          bool
	closeOnce       sync.Once
	seq             uint16
}

// SampleRate returns the audio sample rate required by the camera.
func (s *TalkSession) SampleRate() int {
	return s.sampleRate
}

// SamplesPerBlock returns the required PCM sample count for one ADPCM block.
func (s *TalkSession) SamplesPerBlock() int {
	return s.samplesPerBlock
}

// BytesPerBlock returns the required encoded ADPCM block size, including the 4-byte predictor header.
func (s *TalkSession) BytesPerBlock() int {
	return s.bytesPerBlock
}

// WriteADPCMBlock writes one full ADPCM block to the camera.
func (s *TalkSession) WriteADPCMBlock(_ context.Context, block []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return context.Canceled
	}
	if len(block) != s.bytesPerBlock {
		return fmt.Errorf("unexpected adpcm block size %d, want %d", len(block), s.bytesPerBlock)
	}

	s.seq++
	payload := serializeTalkADPCMBlock(block, s.seq)
	err := s.client.writeRequest(request{
		MsgID:     msgIDTalk,
		ChannelID: s.channel,
		MsgNum:    s.client.reserveMessageNumber(),
		Class:     classModernWithOffset,
		Extension: s.binaryExtension,
		Body:      payload,
		Binary:    true,
	})
	return err
}

// Close stops the active talkback session.
func (s *TalkSession) Close(ctx context.Context) error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		err = s.client.stopTalk(ctx, s.channel)
	})
	return err
}

// StartTalk negotiates a talkback session and returns a writer for ADPCM blocks.
func (c *Client) StartTalk(ctx context.Context, channel uint8) (*TalkSession, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}

	ability, err := c.getTalkAbility(ctx, channel)
	if err != nil {
		return nil, err
	}

	cfg, err := defaultTalkConfig(channel, ability)
	if err != nil {
		return nil, err
	}

	if err := c.startTalkSession(ctx, channel, cfg); err != nil {
		return nil, err
	}

	binaryExtension, err := buildTalkExtension(channel, true)
	if err != nil {
		_ = c.stopTalk(ctx, channel)
		return nil, err
	}

	samplesPerBlock := int(cfg.AudioConfig.LengthPerEncode)
	bytesPerBlock := int(cfg.AudioConfig.LengthPerEncode)/2 + 4

	return &TalkSession{
		client:          c,
		channel:         channel,
		msgNum:          c.reserveMessageNumber(),
		binaryExtension: binaryExtension,
		sampleRate:      int(cfg.AudioConfig.SampleRate),
		samplesPerBlock: samplesPerBlock,
		bytesPerBlock:   bytesPerBlock,
	}, nil
}

func (c *Client) getTalkAbility(ctx context.Context, channel uint8) (*TalkAbility, error) {
	extension, err := buildTalkExtension(channel, false)
	if err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(ctx, request{
		MsgID:     msgIDTalkAbility,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Extension: extension,
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return nil, &UnsupportedTalkError{Reason: fmt.Sprintf("status %d from talk ability", statusErr.Code)}
		}
		return nil, err
	}

	var body talkAbilityBody
	if err := xml.Unmarshal([]byte(resp.XML), &body); err != nil {
		return nil, fmt.Errorf("parse talk ability: %w", err)
	}
	if body.TalkAbility == nil {
		return nil, &UnsupportedTalkError{Reason: "talk ability missing from response"}
	}

	return body.TalkAbility, nil
}

func defaultTalkConfig(channel uint8, ability *TalkAbility) (TalkConfig, error) {
	if ability == nil {
		return TalkConfig{}, &UnsupportedTalkError{Reason: "empty talk ability"}
	}
	if len(ability.DuplexList) == 0 || len(ability.AudioStreamModeList) == 0 || len(ability.AudioConfigList) == 0 {
		return TalkConfig{}, &UnsupportedTalkError{Reason: "camera returned no talk profiles"}
	}

	for _, option := range ability.AudioConfigList {
		cfg := option.AudioConfig
		if cfg.AudioType != "adpcm" {
			continue
		}
		if cfg.SampleRate == 0 || cfg.LengthPerEncode == 0 {
			continue
		}

		version := ability.Version
		if version == "" {
			version = "1.1"
		}

		cfg.Priority = nil

		duplex := ability.DuplexList[0].Duplex
		for _, d := range ability.DuplexList {
			if d.Duplex == "fullDuplex" {
				duplex = "fullDuplex"
				break
			}
		}

		audioMode := ability.AudioStreamModeList[0].AudioStreamMode
		for _, m := range ability.AudioStreamModeList {
			if m.AudioStreamMode == "speaker" {
				audioMode = "speaker"
				break
			}
		}

		return TalkConfig{
			Version:         version,
			ChannelID:       channel,
			Duplex:          duplex,
			AudioStreamMode: audioMode,
			AudioConfig:     cfg,
		}, nil
	}

	return TalkConfig{}, &UnsupportedTalkError{Reason: "camera does not advertise adpcm talk"}
}

func (c *Client) startTalkSession(ctx context.Context, channel uint8, cfg TalkConfig) error {
	extension, err := buildTalkExtension(channel, false)
	if err != nil {
		return err
	}

	body, err := marshalXMLDocument(talkConfigBody{TalkConfig: &cfg})
	if err != nil {
		return err
	}

	req := request{
		MsgID:     msgIDTalkConfig,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Extension: extension,
		Body:      body,
	}

	resp, err := c.roundTripRequest(ctx, req)
	if err != nil {
		return err
	}
	if resp.Header.ResponseCode == 422 {
		if err := c.stopTalk(ctx, channel); err != nil {
			return err
		}

		resp, err = c.roundTripRequest(ctx, req)
		if err != nil {
			return err
		}
	}

	if err := resp.success(); err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return &UnsupportedTalkError{Reason: fmt.Sprintf("talk config rejected with status %d", statusErr.Code)}
		}
		return err
	}

	return nil
}

func (c *Client) stopTalk(ctx context.Context, channel uint8) error {
	extension, err := buildTalkExtension(channel, false)
	if err != nil {
		return err
	}

	_, err = c.sendRequest(ctx, request{
		MsgID:     msgIDTalkReset,
		ChannelID: channel,
		Class:     classModernWithOffset,
		Extension: extension,
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) && statusErr.Code == 422 {
			return nil
		}
		return err
	}
	return nil
}

func buildTalkExtension(channel uint8, binaryData bool) ([]byte, error) {
	ext := talkExtension{
		Version:   "1.1",
		ChannelID: channel,
	}
	if binaryData {
		v := 1
		ext.BinaryData = &v
	}
	return marshalXMLDocument(ext)
}

func serializeTalkADPCMBlock(block []byte, seq uint16) []byte {
	payloadSize := len(block) + 4
	total := 8 + payloadSize + padLen(payloadSize)

	out := make([]byte, total)
	binary.LittleEndian.PutUint32(out[0:4], bcmediaADPCM)
	binary.LittleEndian.PutUint16(out[4:6], uint16(payloadSize)) //#nosec G115
	binary.LittleEndian.PutUint16(out[6:8], uint16(payloadSize)) //#nosec G115
	binary.LittleEndian.PutUint16(out[8:10], bcmediaADPCMHeader)
	binary.LittleEndian.PutUint16(out[10:12], seq)
	copy(out[12:], block)
	return out
}
