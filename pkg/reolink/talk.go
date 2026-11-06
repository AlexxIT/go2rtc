package reolink

import (
	"fmt"
	"strconv"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type talkParams struct {
	duplex          string
	streamMode      string
	audioType       string
	soundTrack      string
	sampleRate      int
	precision       int
	samplesPerBlock int
}

// parseTalkAbility takes the first audioConfig of the TalkAbility reply
func parseTalkAbility(xml string) (talkParams, error) {
	cfg := talkParams{
		duplex:     core.Between(xml, "<duplex>", "<"),
		streamMode: core.Between(xml, "<audioStreamMode>", "<"),
		audioType:  core.Between(xml, "<audioType>", "<"),
		soundTrack: core.Between(xml, "<soundTrack>", "<"),
	}
	if cfg.audioType != "adpcm" {
		return cfg, fmt.Errorf("reolink: unsupported talk audio %q", cfg.audioType)
	}

	var err error
	if cfg.sampleRate, err = strconv.Atoi(core.Between(xml, "<sampleRate>", "<")); err != nil {
		return cfg, fmt.Errorf("reolink: wrong sampleRate: %w", err)
	}
	if cfg.precision, err = strconv.Atoi(core.Between(xml, "<samplePrecision>", "<")); err != nil {
		return cfg, fmt.Errorf("reolink: wrong samplePrecision: %w", err)
	}
	if cfg.samplesPerBlock, err = strconv.Atoi(core.Between(xml, "<lengthPerEncoder>", "<")); err != nil {
		return cfg, fmt.Errorf("reolink: wrong lengthPerEncoder: %w", err)
	}
	if cfg.sampleRate <= 0 {
		return cfg, fmt.Errorf("reolink: wrong sampleRate: %d", cfg.sampleRate)
	}
	if cfg.precision != 16 {
		return cfg, fmt.Errorf("reolink: unsupported samplePrecision: %d", cfg.precision)
	}
	if cfg.samplesPerBlock <= 0 || cfg.samplesPerBlock%2 != 0 {
		return cfg, fmt.Errorf("reolink: wrong lengthPerEncoder: %d", cfg.samplesPerBlock)
	}
	return cfg, nil
}

// talkConfig must match the camera's own formatting byte for byte (an element per line, a space
// before "?>", no priority element), otherwise the camera refuses it with status 400
func (p talkParams) talkConfig(channel int) string {
	return xmlHeader + "<body>\n" +
		"<TalkConfig version=\"1.1\">\n" +
		"<channelId>" + strconv.Itoa(channel) + "</channelId>\n" +
		"<duplex>" + p.duplex + "</duplex>\n" +
		"<audioStreamMode>" + p.streamMode + "</audioStreamMode>\n" +
		"<audioConfig>\n" +
		"<audioType>" + p.audioType + "</audioType>\n" +
		"<sampleRate>" + strconv.Itoa(p.sampleRate) + "</sampleRate>\n" +
		"<samplePrecision>" + strconv.Itoa(p.precision) + "</samplePrecision>\n" +
		"<lengthPerEncoder>" + strconv.Itoa(p.samplesPerBlock) + "</lengthPerEncoder>\n" +
		"<soundTrack>" + p.soundTrack + "</soundTrack>\n" +
		"</audioConfig>\n" +
		"</TalkConfig>\n" +
		"</body>\n"
}

func channelExt(channel int) string {
	return xmlHeader + "<Extension version=\"1.1\">\n<channelId>" + strconv.Itoa(channel) + "</channelId>\n</Extension>\n"
}

func talkExt(channel int) string {
	return xmlHeader + "<Extension version=\"1.1\">\n<binaryData>1</binaryData>\n" +
		"<channelId>" + strconv.Itoa(channel) + "</channelId>\n</Extension>\n"
}
