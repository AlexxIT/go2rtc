package baichuan

import (
	"encoding/xml"
	"fmt"
	"strings"
)

const (
	maxTalkAbilityBody    = 64 << 10
	maxTalkAbilityOptions = 32
	maxTalkAbilityText    = 64
)

type talkAudioConfig struct {
	Priority        *uint32 `xml:"priority,omitempty"`
	AudioType       string  `xml:"audioType"`
	SampleRate      uint32  `xml:"sampleRate"`
	SamplePrecision uint32  `xml:"samplePrecision"`
	SamplesPerBlock uint32  `xml:"lengthPerEncoder"`
	SoundTrack      string  `xml:"soundTrack"`
}

type talkAbility struct {
	Version  string       `xml:"version,attr"`
	Duplexes []talkDuplex `xml:"duplexList"`
	Modes    []talkMode   `xml:"audioStreamModeList"`
	Configs  []struct {
		Value talkAudioConfig `xml:"audioConfig"`
	} `xml:"audioConfigList"`
}

type talkDuplex struct {
	Value string `xml:"duplex"`
}

type talkMode struct {
	Value string `xml:"audioStreamMode"`
}

type talkAbilityEnvelope struct {
	XMLName xml.Name     `xml:"body"`
	Ability *talkAbility `xml:"TalkAbility"`
}

type talkConfig struct {
	Version string          `xml:"version,attr"`
	Channel uint8           `xml:"channelId"`
	Duplex  string          `xml:"duplex"`
	Mode    string          `xml:"audioStreamMode"`
	Audio   talkAudioConfig `xml:"audioConfig"`
}

type talkConfigEnvelope struct {
	XMLName xml.Name   `xml:"body"`
	Config  talkConfig `xml:"TalkConfig"`
}

type talkExtension struct {
	XMLName xml.Name `xml:"Extension"`
	Version string   `xml:"version,attr"`
	Channel uint8    `xml:"channelId"`
	Binary  *uint8   `xml:"binaryData,omitempty"`
}

func decodeTalkAbility(b []byte) (*talkAbility, error) {
	if len(b) > maxTalkAbilityBody {
		return nil, fmt.Errorf("talk ability XML exceeds %d bytes", maxTalkAbilityBody)
	}
	var envelope talkAbilityEnvelope
	if err := xml.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	if envelope.Ability == nil {
		return nil, &UnsupportedTalkError{Reason: "ability missing"}
	}
	ability := envelope.Ability
	if len(ability.Duplexes) > maxTalkAbilityOptions || len(ability.Modes) > maxTalkAbilityOptions ||
		len(ability.Configs) > maxTalkAbilityOptions || len(ability.Version) > maxTalkAbilityText {
		return nil, fmt.Errorf("talk ability exceeds semantic limits")
	}
	for _, option := range ability.Duplexes {
		if len(option.Value) > maxTalkAbilityText {
			return nil, fmt.Errorf("talk ability exceeds semantic limits")
		}
	}
	for _, option := range ability.Modes {
		if len(option.Value) > maxTalkAbilityText {
			return nil, fmt.Errorf("talk ability exceeds semantic limits")
		}
	}
	for _, option := range ability.Configs {
		if len(option.Value.AudioType) > maxTalkAbilityText || len(option.Value.SoundTrack) > maxTalkAbilityText {
			return nil, fmt.Errorf("talk ability exceeds semantic limits")
		}
	}
	return envelope.Ability, nil
}

func selectTalkConfig(channel uint8, ability *talkAbility) (talkConfig, error) {
	if len(ability.Duplexes) == 0 || len(ability.Modes) == 0 {
		return talkConfig{}, &UnsupportedTalkError{Reason: "incomplete ability"}
	}
	var audio talkAudioConfig
	var adpcm bool
	for _, option := range ability.Configs {
		if !strings.EqualFold(option.Value.AudioType, "adpcm") {
			continue
		}
		adpcm = true
		if validTalkAudio(option.Value) {
			audio = option.Value
			break
		}
	}
	if !adpcm {
		return talkConfig{}, &UnsupportedTalkError{Reason: "ADPCM profile missing"}
	}
	if audio.AudioType == "" {
		return talkConfig{}, &UnsupportedTalkError{Reason: "invalid ADPCM profile"}
	}
	audio.Priority = nil
	version := ability.Version
	if version == "" {
		version = "1.1"
	}
	duplex := ability.Duplexes[0].Value
	for _, option := range ability.Duplexes {
		if strings.EqualFold(option.Value, "fullDuplex") {
			duplex = option.Value
			break
		}
	}
	mode := ability.Modes[0].Value
	for _, option := range ability.Modes {
		if strings.EqualFold(option.Value, "speaker") {
			mode = option.Value
			break
		}
	}
	return talkConfig{
		Version: version, Channel: channel, Duplex: duplex, Mode: mode, Audio: audio,
	}, nil
}

func validTalkAudio(audio talkAudioConfig) bool {
	return audio.SampleRate >= 8000 && audio.SampleRate <= 48000 && audio.SamplePrecision == 16 &&
		audio.SamplesPerBlock >= 2 && audio.SamplesPerBlock <= 8192 && audio.SamplesPerBlock&1 == 0 &&
		(audio.SoundTrack == "" || strings.EqualFold(audio.SoundTrack, "mono"))
}

func buildTalkExtension(channel uint8, binaryData bool) ([]byte, error) {
	extension := talkExtension{Version: "1.1", Channel: channel}
	if binaryData {
		value := uint8(1)
		extension.Binary = &value
	}
	return marshalDocument(extension)
}
