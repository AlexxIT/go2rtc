package reolink

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const (
	maxProbeBytes    = 32 << 20
	maxProbePackets  = 4096
	maxTimestampStep = uint32(30 * time.Second / time.Microsecond)
	// Confirmation may bridge two normal clock intervals, never an unbounded reset.
	maxRecoveryTimestampStep = 2 * maxTimestampStep
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
	packet    *rtp.Packet
	track     mediaTrack
	malformed bool
	converted bool
	samples   uint32
}

func (f mediaFrame) valid() bool {
	return f.packet != nil
}

type mediaPipeline struct {
	medias       []*core.Media
	epoch        mediaEpoch
	sharedClock  bool
	videoResyncs *atomic.Uint64

	videoCodec     string
	videoParams    string
	videoCandidate string
	videoResync    bool
	videoMedia     *core.Media
	videoTS        mediaClock
	videoOnly      bool
	videoPending   videoClockCandidate
	aacGap         uint64 // Full-width accepted I time while AAC remains behind.

	audioKind   baichuan.MediaKind
	audioConfig uint16
	audioMedia  *core.Media
	audioTS     uint64
	audioRate   uint32
	audioSSRC   uint32
	audioSeq    uint16
}

func (m *mediaPipeline) probeVideo(packet baichuan.MediaPacket) error {
	if packet.Codec != "H264" && packet.Codec != "H265" {
		return mediaErrorf("reolink: unsupported video codec %q", packet.Codec)
	}
	payload := encodeAnnexBToAVCC(packet.Codec, packet.Data)
	if !validVideoConfig(packet.Codec, payload) {
		return nil
	}
	return m.configureVideoPayload(packet.Codec, payload)
}

func (m *mediaPipeline) configureVideoPayload(name string, payload []byte) error {
	params, err := videoParameterSignature(name, payload)
	if err != nil {
		return err
	}
	var codec *core.Codec
	switch name {
	case "H264":
		codec = &core.Codec{
			Name: core.CodecH264, ClockRate: 90000, PayloadType: core.PayloadTypeRAW,
			FmtpLine: h264Fmtp(payload),
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
	m.videoParams = params
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
		codec.PayloadType = 96
		m.audioConfig = config
		m.audioSSRC = rand.Uint32()
		m.audioSeq = uint16(rand.Uint32())
		if m.epoch.aac.valid && m.epoch.aac.config == config {
			m.audioSeq = m.epoch.aac.next
		}
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
	if m.sharedClock {
		m.audioTS = rescaleTimestampCeil(m.epoch.video, 90000, codec.ClockRate)
	}
	if m.epoch.audioSet {
		// The saved audio epoch is the next sample, not the last emitted sample.
		m.audioTS = max(m.audioTS, rescaleTimestampCeil(m.epoch.audio, m.epoch.audioRate, codec.ClockRate))
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
	if m.videoTS.set && (packet.Timestamp-m.videoTS.last >= 1<<31 ||
		m.videoResync && packet.Timestamp == m.videoTS.last) {
		m.videoResync = true
		m.videoPending = videoClockCandidate{}
		return mediaFrame{}, nil
	}
	if packet.ShortVideo {
		m.suppressInvalidVideo()
		return mediaFrame{malformed: true}, nil
	}
	payload := encodeOwnedToAVCC(packet.Codec, packet.Data)
	info, err := inspectVideoPayload(packet.Codec, payload)
	if err != nil {
		m.suppressInvalidVideo()
		return mediaFrame{malformed: true, converted: len(payload) != 0 && &payload[0] == &packet.Data[0]}, nil
	}
	if packet.Kind == baichuan.MediaVideoI {
		if !info.keyframe || !info.fullParameters &&
			(info.hasParameters || m.videoResync || m.videoParams == "") {
			m.suppressInvalidVideo()
			return mediaFrame{}, nil
		}
	} else if info.hasParameters && packet.Codec == "H264" && !h264ParametersKnown(payload, m.videoParams) {
		// Only complete keyframes can confirm a replacement configuration.
		m.videoPending = videoClockCandidate{}
		m.videoResync = true
		return mediaFrame{}, nil
	}
	if packet.Kind == baichuan.MediaVideoI && info.fullParameters {
		same, candidate, err := m.videoParameters(packet.Codec, payload)
		if err != nil {
			m.suppressInvalidVideo()
			return mediaFrame{}, nil
		}
		if !same {
			m.videoPending = videoClockCandidate{}
			if candidate == m.videoCandidate {
				return mediaFrame{}, mediaErrorf("reolink: %s parameter sets changed", packet.Codec)
			}
			m.videoCandidate = candidate
			m.videoResync = true
			return mediaFrame{}, nil
		}
		m.videoCandidate = ""
	}
	if m.videoPending.set {
		delta := packet.Timestamp - m.videoPending.last
		if delta >= 1<<31 || delta > maxTimestampStep || packet.Kind == baichuan.MediaVideoP && info.hasParameters {
			m.videoPending = videoClockCandidate{}
		} else {
			m.videoPending.last = packet.Timestamp
		}
	}
	if m.videoResync && (packet.Kind != baichuan.MediaVideoI || !info.fullParameters) {
		return mediaFrame{}, nil
	}
	timestamp, err := m.videoTS.next(packet.Timestamp, 90000)
	if err != nil {
		var accepted bool
		timestamp, accepted, err = m.recoverVideoTimestamp(packet.Timestamp, err)
		if err != nil {
			return mediaFrame{}, mediaErrorf("reolink: video timestamp: %w", err)
		}
		if !accepted {
			return mediaFrame{}, nil
		}
	}
	var gapErr error
	if skew, ahead := m.videoAheadOfAAC(); ahead {
		if packet.Kind == baichuan.MediaVideoI {
			if m.aacGap == 0 {
				m.aacGap = m.videoTS.value
			} else if m.videoTS.value != m.aacGap {
				gapErr = mediaErrorf("reolink: video ahead of AAC by %d ticks", skew)
			}
		}
	} else {
		m.aacGap = 0
	}
	if m.videoResync {
		m.videoResync = false
		if m.videoResyncs != nil {
			m.videoResyncs.Add(1)
		}
	}
	m.videoPending = videoClockCandidate{}
	return mediaFrame{
		track:  trackVideo,
		packet: &rtp.Packet{Header: rtp.Header{Timestamp: timestamp}, Payload: payload},
	}, gapErr
}

func (m *mediaPipeline) videoAheadOfAAC() (uint64, bool) {
	if !m.sharedClock || m.audioKind != baichuan.MediaAAC || m.audioRate == 0 {
		return 0, false
	}
	audio := rescaleTimestamp(m.audioTS, m.audioRate, 90000)
	if m.videoTS.value <= audio {
		return 0, false
	}
	skew := m.videoTS.value - audio
	return skew, skew > 90000
}

func (m *mediaPipeline) suppressInvalidVideo() {
	m.videoCandidate = ""
	m.videoResync = true
	m.videoPending = videoClockCandidate{}
}

type videoClockCandidate struct {
	first uint32
	last  uint32
	set   bool
}

func (m *mediaPipeline) recoverVideoTimestamp(raw uint32, err error) (uint32, bool, error) {
	delta := raw - m.videoTS.last
	if m.videoOnly && m.videoResync && m.videoTS.set &&
		delta > maxTimestampStep && delta <= maxRecoveryTimestampStep {
		if !m.videoPending.set {
			m.videoPending = videoClockCandidate{first: raw, last: raw, set: true}
			return 0, false, nil
		}
		if raw == m.videoPending.first {
			return 0, false, nil
		}
		return m.videoTS.advance(raw, 90000), true, nil
	}
	return 0, false, err
}

func (m *mediaPipeline) videoParameters(codec string, payload []byte) (same bool, candidate string, err error) {
	params, err := videoParameterSignature(codec, payload)
	if err != nil {
		return false, "", err
	}
	return params == m.videoParams, params, nil
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
		packet:  &rtp.Packet{Header: rtp.Header{Timestamp: uint32(m.audioTS)}, Payload: payload},
		samples: samples,
	}
	if packet.Kind == baichuan.MediaAAC {
		// Emit RTP here so RTSP preserves our audio epoch instead of recounting samples.
		frame.packet.Header.Version = 2
		frame.packet.Header.Marker = true
		frame.packet.Header.PayloadType = m.audioMedia.Codecs[0].PayloadType
		frame.packet.Header.SSRC = m.audioSSRC
		frame.packet.Header.SequenceNumber = m.audioSeq
		frame.packet.Payload = make([]byte, 4+len(payload))
		frame.packet.Payload[1] = 16
		binary.BigEndian.PutUint16(frame.packet.Payload[2:], uint16(len(payload))<<3)
		copy(frame.packet.Payload[4:], payload)
		m.audioSeq++
	}
	m.audioTS += uint64(samples)
	if m.aacGap != 0 {
		if _, ahead := m.videoAheadOfAAC(); !ahead {
			m.aacGap = 0
		}
	}
	return frame, nil
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
	value     uint64
	remainder uint64
}

func (c *mediaClock) next(timestamp, rate uint32) (uint32, error) {
	if !c.set {
		c.set = true
		c.last = timestamp
		return uint32(c.value), nil
	}
	delta := timestamp - c.last
	if delta >= 1<<31 || delta > maxTimestampStep {
		return uint32(c.value), fmt.Errorf("camera clock discontinuity: %d to %d microseconds", c.last, timestamp)
	}
	return c.advance(timestamp, rate), nil
}

func (c *mediaClock) advance(timestamp, rate uint32) uint32 {
	scaled := uint64(timestamp-c.last)*uint64(rate) + c.remainder
	c.last = timestamp
	c.value += scaled / 1_000_000
	c.remainder = scaled % 1_000_000
	return uint32(c.value)
}

func (c *mediaClock) current() (uint64, bool) {
	return c.value, c.set
}

func nowTimestamp(rate uint32) uint64 {
	now := uint64(time.Now().UnixNano())
	return now/uint64(time.Second)*uint64(rate) + now%uint64(time.Second)*uint64(rate)/uint64(time.Second)
}
