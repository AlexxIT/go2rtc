package reolink

import (
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const (
	maxProbeBytes    = 32 << 20
	maxProbePackets  = 4096
	maxTimestampStep = uint32(30 * time.Second / time.Microsecond)
)

type mediaError struct {
	err error
}

func (e *mediaError) Error() string {
	return e.err.Error()
}

func (e *mediaError) Unwrap() error {
	return e.err
}

func mediaErrorf(format string, args ...any) error {
	return &mediaError{err: fmt.Errorf(format, args...)}
}

type mediaTrack uint8

const (
	trackVideo mediaTrack = iota
	trackAudio
)

type mediaFrame struct {
	packet  *rtp.Packet
	track   mediaTrack
	samples uint32
}

func (f mediaFrame) valid() bool {
	return f.packet != nil
}

type mediaPipeline struct {
	medias []*core.Media
	epoch  mediaEpoch

	videoCodec     string
	videoConfig    string
	videoParams    string
	videoCandidate string
	videoResync    bool
	videoMedia     *core.Media
	videoTS        mediaClock

	audioKind   baichuan.MediaKind
	audioConfig uint16
	audioMedia  *core.Media
	audioTS     uint32
	audioRate   uint32
}

func (m *mediaPipeline) configureVideo(packet baichuan.MediaPacket) error {
	payload := annexb.EncodeToAVCC(packet.Data)
	if !validVideoConfig(packet.Codec, payload) {
		return mediaErrorf("reolink: invalid %s keyframe", packet.Codec)
	}
	return m.configureVideoPayload(packet.Codec, payload)
}

func (m *mediaPipeline) probeVideo(packet baichuan.MediaPacket) error {
	payload := annexb.EncodeToAVCC(packet.Data)
	if packet.Codec != "H264" && packet.Codec != "H265" {
		return mediaErrorf("reolink: unsupported video codec %q", packet.Codec)
	}
	if !validVideoConfig(packet.Codec, payload) {
		return nil
	}
	return m.configureVideoPayload(packet.Codec, payload)
}

func (m *mediaPipeline) configureVideoPayload(name string, payload []byte) error {
	var codec *core.Codec
	switch name {
	case "H264":
		params, err := h264ParameterSignature(payload)
		if err != nil {
			return err
		}
		m.videoParams = params
		codec = &core.Codec{
			Name: core.CodecH264, ClockRate: 90000, PayloadType: core.PayloadTypeRAW,
			FmtpLine: h264.GetFmtpLine(payload),
		}
	case "H265":
		codec = h265.AVCCToCodec(payload)
		if codec == nil {
			return mediaErrorf("reolink: invalid H265 keyframe")
		}
	default:
		return mediaErrorf("reolink: unsupported video codec %q", name)
	}
	m.videoMedia = &core.Media{
		Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{codec},
	}
	m.videoCodec = name
	m.videoConfig = codec.FmtpLine
	m.videoTS.value = m.epoch.video
	m.medias = append(m.medias, m.videoMedia)
	return nil
}

func (m *mediaPipeline) configureAudio(packet baichuan.MediaPacket) error {
	var codec *core.Codec
	switch packet.Kind {
	case baichuan.MediaAAC:
		codec = aac.ADTSToCodec(packet.Data)
		config, ok := aacConfig(packet.Data)
		if codec == nil || codec.ClockRate == 0 || codec.Channels == 0 || !ok {
			return mediaErrorf("reolink: invalid AAC header")
		}
		codec.PayloadType = core.PayloadTypeRAW
		m.audioConfig = config
	case baichuan.MediaADPCM:
		if _, err := baichuan.DecodeADPCMBlock(packet.Data); err != nil {
			return mediaErrorf("reolink: invalid ADPCM audio: %w", err)
		}
		codec = &core.Codec{
			Name: core.CodecPCMA, ClockRate: 8000, Channels: 1, PayloadType: core.PayloadTypeRAW,
		}
	default:
		return mediaErrorf("reolink: unsupported audio kind %d", packet.Kind)
	}
	m.audioMedia = &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{codec},
	}
	m.audioKind = packet.Kind
	m.audioTS = nowTimestamp(codec.ClockRate)
	if m.epoch.audioSet && m.epoch.audioRate == codec.ClockRate {
		m.audioTS = nextTimestampEpoch(m.audioTS, m.epoch.audio, true)
	}
	m.audioRate = codec.ClockRate
	m.medias = append(m.medias, m.audioMedia)
	return nil
}

func (m *mediaPipeline) frame(packet baichuan.MediaPacket) (mediaFrame, error) {
	switch packet.Kind {
	case baichuan.MediaVideoI, baichuan.MediaVideoP:
		return m.videoFrame(packet)
	case baichuan.MediaAAC, baichuan.MediaADPCM:
		return m.audioFrame(packet)
	default:
		return mediaFrame{}, nil
	}
}

func (m *mediaPipeline) videoFrame(packet baichuan.MediaPacket) (mediaFrame, error) {
	if packet.Codec != m.videoCodec {
		return mediaFrame{}, mediaErrorf("reolink: video codec changed from %s to %s", m.videoCodec, packet.Codec)
	}
	payload := encodeOwnedToAVCC(packet.Data)
	config, err := inspectVideoPayload(packet.Codec, payload)
	if err != nil {
		m.suppressInvalidVideo()
		return mediaFrame{}, nil
	}
	if packet.Kind == baichuan.MediaVideoI {
		if !config {
			m.suppressInvalidVideo()
			return mediaFrame{}, nil
		}
		same, candidate, err := m.videoParameters(packet.Codec, payload)
		if err != nil {
			m.suppressInvalidVideo()
			return mediaFrame{}, nil
		}
		if !same {
			if candidate == m.videoCandidate {
				return mediaFrame{}, mediaErrorf("reolink: %s parameter sets changed", packet.Codec)
			}
			m.videoCandidate = candidate
			m.videoResync = true
			return mediaFrame{}, nil
		}
		m.videoCandidate = ""
		if m.videoResync {
			m.videoResync = false
		}
	}
	if m.videoResync {
		return mediaFrame{}, nil
	}
	timestamp, err := m.videoTS.next(packet.Timestamp, 90000)
	if err != nil {
		return mediaFrame{}, mediaErrorf("reolink: video timestamp: %w", err)
	}
	return mediaFrame{
		track:  trackVideo,
		packet: &rtp.Packet{Header: rtp.Header{Timestamp: timestamp}, Payload: payload},
	}, nil
}

func (m *mediaPipeline) suppressInvalidVideo() {
	m.videoCandidate = ""
	m.videoResync = true
}

func (m *mediaPipeline) videoParameters(codec string, payload []byte) (same bool, candidate string, err error) {
	if codec == "H264" {
		params, sigErr := h264ParameterSignature(payload)
		if sigErr != nil {
			return false, "", sigErr
		}
		return params == m.videoParams, params, nil
	}
	parsed := h265.AVCCToCodec(payload)
	if parsed == nil {
		return false, "", mediaErrorf("reolink: invalid H265 keyframe")
	}
	return parsed.FmtpLine == m.videoConfig, parsed.FmtpLine, nil
}

func (m *mediaPipeline) audioFrame(packet baichuan.MediaPacket) (mediaFrame, error) {
	if m.audioKind == 0 {
		return mediaFrame{}, nil
	}
	if packet.Kind != m.audioKind {
		return mediaFrame{}, mediaErrorf("reolink: audio kind changed from %d to %d", m.audioKind, packet.Kind)
	}
	if packet.Kind == baichuan.MediaAAC {
		config, ok := aacConfig(packet.Data)
		if !ok {
			return mediaFrame{}, mediaErrorf("reolink: invalid AAC packet")
		}
		if config != m.audioConfig {
			return mediaFrame{}, mediaErrorf("reolink: AAC configuration changed")
		}
	}
	payload, samples, err := audioPayload(packet)
	if err != nil {
		return mediaFrame{}, err
	}
	frame := mediaFrame{
		track:   trackAudio,
		packet:  &rtp.Packet{Header: rtp.Header{Timestamp: m.audioTS}, Payload: payload},
		samples: samples,
	}
	m.audioTS += samples
	return frame, nil
}

type mediaProbe struct {
	packets []baichuan.MediaPacket
	bytes   int
}

func (p *mediaProbe) retain(packet baichuan.MediaPacket) error {
	if packet.Kind == baichuan.MediaVideoI {
		p.reset()
	}
	if len(packet.Data) > maxProbeBytes-p.bytes {
		return mediaErrorf("reolink: codec probe exceeds %d bytes", maxProbeBytes)
	}
	if len(p.packets) >= maxProbePackets {
		return mediaErrorf("reolink: codec probe exceeds %d packets", maxProbePackets)
	}
	packet.Data = append([]byte(nil), packet.Data...)
	p.packets = append(p.packets, packet)
	p.bytes += len(packet.Data)
	return nil
}

func (p *mediaProbe) reset() {
	clear(p.packets)
	p.packets = p.packets[:0]
	p.bytes = 0
}

func (p *mediaProbe) release() {
	p.reset()
	p.packets = nil
}

func aacConfig(data []byte) (uint16, bool) {
	if !aac.IsADTS(data) {
		return 0, false
	}
	return uint16(data[2]&0xfd)<<8 | uint16(data[3]&0xc0), true
}

func audioPayload(packet baichuan.MediaPacket) ([]byte, uint32, error) {
	if packet.Kind == baichuan.MediaADPCM {
		samples, err := baichuan.DecodeADPCMBlock(packet.Data)
		if err != nil {
			return nil, 0, mediaErrorf("reolink: invalid ADPCM packet: %w", err)
		}
		payload := make([]byte, len(samples))
		for i, sample := range samples {
			payload[i] = pcm.PCMtoPCMA(sample)
		}
		return payload, uint32(len(samples)), nil
	}
	if packet.Kind != baichuan.MediaAAC || !aac.IsADTS(packet.Data) {
		return nil, 0, mediaErrorf("reolink: invalid AAC packet")
	}
	size := int(aac.ReadADTSSize(packet.Data))
	header := aac.ADTSHeaderLen(packet.Data)
	if size != len(packet.Data) || header >= size {
		return nil, 0, mediaErrorf("reolink: invalid AAC packet size")
	}
	samples := uint32(packet.Data[6]&3+1) * 1024
	return packet.Data[header:size], samples, nil
}

type mediaClock struct {
	set       bool
	last      uint32
	value     uint32
	remainder uint64
}

func (c *mediaClock) next(timestamp, rate uint32) (uint32, error) {
	if !c.set {
		c.set = true
		c.last = timestamp
		return c.value, nil
	}
	delta := timestamp - c.last
	if delta >= 1<<31 || delta > maxTimestampStep {
		return c.value, fmt.Errorf("camera clock discontinuity: %d to %d microseconds", c.last, timestamp)
	}
	c.last = timestamp
	scaled := uint64(delta)*uint64(rate) + c.remainder
	c.value += uint32(scaled / 1_000_000)
	c.remainder = scaled % 1_000_000
	return c.value, nil
}

func (c *mediaClock) current() (uint32, bool) {
	return c.value, c.set
}

func nowTimestamp(rate uint32) uint32 {
	now := uint64(time.Now().UnixNano())
	return uint32(now/uint64(time.Second)*uint64(rate) +
		now%uint64(time.Second)*uint64(rate)/uint64(time.Second))
}
