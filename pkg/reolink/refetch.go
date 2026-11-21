package reolink

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

const (
	refetchSeconds = 25
	refetchTimeout = 8 * time.Second
	refetchTail    = 16
)

var errRefetchContinuity = errors.New("reolink: history could not establish media continuity")

type refetchMark struct {
	digest [sha256.Size]byte
	value  uint32 // Video camera timestamp or AAC sample count.
	track  mediaTrack
}

// Keep the ordered tail of actual handoffs, never the media itself.
type refetchHistory struct {
	refetchMarks
	videoAt  time.Time
	raw      uint32
	step     uint32
	attempts atomic.Uint64
	resumes  atomic.Uint64
}

type refetchMarks struct {
	marks [refetchTail]refetchMark
	next  int
	count int
}

func (h *refetchMarks) add(mark refetchMark) {
	h.marks[h.next] = mark
	h.next = (h.next + 1) % len(h.marks)
	if h.count < len(h.marks) {
		h.count++
	}
}

func (h *refetchHistory) record(frame mediaFrame, raw uint32, kind baichuan.MediaKind) {
	payload := frame.packet.Payload
	if frame.track == trackVideo {
		if !h.videoAt.IsZero() {
			h.step = raw - h.raw
		}
		h.raw = raw
		h.videoAt = time.Now()
	} else {
		if kind != baichuan.MediaAAC {
			return
		}
		payload = payload[4:]
		raw = frame.samples
	}
	h.add(refetchMark{sha256.Sum256(payload), raw, frame.track})
}

func (h *refetchMarks) matches(other *refetchMarks) bool {
	if h.count != refetchTail || other.count != refetchTail {
		return false
	}
	for i := range h.marks {
		if h.marks[(h.next+i)%refetchTail] != other.marks[(other.next+i)%refetchTail] {
			return false
		}
	}
	return true
}

func (p *Producer) canRefetch(err error) bool {
	if err == nil || errors.Is(err, errAudioRateBelowExpected) || p.history == nil || p.ctx.Err() != nil ||
		p.stopCtx.Err() != nil || time.Since(p.history.videoAt) >= refetchSeconds*time.Second ||
		p.pipeline.aacGap != 0 {
		return false
	}
	video, audio := false, false
	for _, mark := range p.history.marks {
		video = video || mark.track == trackVideo
		audio = audio || mark.track == trackAudio
	}
	return p.history.count == refetchTail && video && (p.audio == nil || audio)
}

func (p *Producer) refetch(old profileInput, cause error) (profileInput, error) {
	p.history.attempts.Add(1)
	// The original preview must release its camera slot before opening history.
	if err := old.Abort(); err != nil {
		p.logRefetch(false, err)
		return nil, cause
	}
	ctx, cancel := context.WithTimeout(p.ctx, refetchTimeout)
	defer cancel()
	stop := context.AfterFunc(p.stopCtx, cancel)
	defer stop()
	if p.stopCtx.Err() != nil {
		return nil, cause
	}
	s := *p.source
	s.preRecord = refetchSeconds
	input, err := p.open(ctx, p.camera, s)
	if err != nil {
		p.logRefetch(false, err)
		return nil, cause
	}
	cadence := p.history.cadence()
	probe, err := p.readHistory(ctx, input, cadence)
	p.mu.Lock()
	if err != nil || ctx.Err() != nil || p.stopCtx.Err() != nil {
		p.mu.Unlock()
		err = errors.Join(err, ctx.Err(), p.stopCtx.Err(), input.Abort())
		p.logRefetch(false, err)
		return nil, cause
	}
	input = &refetchInput{
		profileInput: input,
		packets:      probe.packets,
		raw:          p.history.raw,
		cadence:      cadence,
		releasedAt:   time.Now(),
	}
	p.input = input
	p.mu.Unlock()
	p.history.resumes.Add(1)
	p.logRefetch(true, nil)
	return input, nil
}

func (p *Producer) logRefetch(resumed bool, err error) {
	p.camera.registry.log.Info().Str("camera", p.camera.remote).
		Uint8("channel", p.key.Channel).Str("profile", string(p.key.Stream)).
		Uint64("generation", p.generation).Bool("resumed", resumed).
		Str("error_class", errorClass(err)).Msg("reolink history refetch")
}

func (p *Producer) historyMark(packet baichuan.MediaPacket) (refetchMark, bool) {
	switch packet.Kind {
	case baichuan.MediaAAC:
		config, ok := aacConfig(packet.Data)
		payload, samples, err := audioPayload(packet)
		if !ok || config != p.pipeline.audioConfig || err != nil {
			return refetchMark{}, false
		}
		return refetchMark{sha256.Sum256(payload), samples, trackAudio}, true
	case baichuan.MediaVideoI, baichuan.MediaVideoP:
		if packet.ShortVideo || packet.Codec != p.pipeline.videoCodec {
			return refetchMark{}, false
		}
		payload := encodeOwnedToAVCC(packet.Codec, packet.Data)
		info, err := inspectVideoPayload(packet.Codec, payload)
		if err != nil {
			return refetchMark{}, false
		}
		if info.hasParameters {
			params, err := videoParameterSignature(packet.Codec, payload)
			if err != nil || params != p.pipeline.videoParams {
				return refetchMark{}, false
			}
		}
		return refetchMark{sha256.Sum256(payload), packet.Timestamp, trackVideo}, true
	default:
		return refetchMark{}, false
	}
}

func (p *Producer) readHistory(ctx context.Context, input profileInput, cadence refetchCadence) (mediaProbe, error) {
	var candidate refetchMarks
	var replay mediaProbe
	m := &p.pipeline
	// The validation pass has private clock state and no output or diagnostics.
	trial := mediaPipeline{
		sharedClock: m.sharedClock, videoOnly: m.videoOnly,
		videoCodec: m.videoCodec, videoParams: m.videoParams,
		videoTS: m.videoTS, videoResync: m.videoResync,
		videoCandidate: m.videoCandidate, videoPending: m.videoPending,
		aacGap:    m.aacGap,
		audioKind: m.audioKind, audioConfig: m.audioConfig, audioMedia: m.audioMedia,
		audioTS: m.audioTS, audioRate: m.audioRate, audioSSRC: m.audioSSRC, audioSeq: m.audioSeq,
	}
	anchored, video, audio := false, false, p.audio == nil
	var bytes int
	for i := 0; i < maxProbePackets; i++ {
		packet, err := input.Read(ctx)
		if err != nil {
			return mediaProbe{}, err
		}
		if len(packet.Data) > maxProbeBytes-bytes {
			break
		}
		bytes += len(packet.Data)
		if packet.Kind == baichuan.MediaInfo {
			p.record(packet)
			continue
		}
		if !anchored {
			p.record(packet)
			if mark, ok := p.historyMark(packet); ok && p.acceptPacket(packet) {
				candidate.add(mark)
				anchored = p.history.matches(&candidate)
			} else if packet.Kind == baichuan.MediaDiscontinuity || p.acceptPacket(packet) {
				candidate = refetchMarks{}
			}
			continue
		}
		if packet.Kind == baichuan.MediaDiscontinuity {
			break
		}
		if !p.acceptPacket(packet) {
			continue
		}
		frame, err := trial.refetchFrame(packet, &cadence)
		if err != nil || !frame.valid() {
			break
		}
		replay.packets = append(replay.packets, packet)
		video = video || frame.track == trackVideo
		audio = audio || frame.track == trackAudio
		if video && audio && trial.refetchAligned(&p.pipeline) {
			return replay, nil
		}
	}
	return mediaProbe{}, errRefetchContinuity
}

func (m *mediaPipeline) refetchFrame(packet baichuan.MediaPacket, cadence *refetchCadence) (mediaFrame, error) {
	if packet.Kind == baichuan.MediaVideoI || packet.Kind == baichuan.MediaVideoP {
		step := packet.Timestamp - m.videoTS.last
		if !cadence.advance(packet.Kind, step) {
			return mediaFrame{}, errRefetchContinuity
		}
		// Trial conversion may change the slice in place. Replay owns the original.
		packet.Data = append([]byte(nil), packet.Data...)
	}
	return m.frame(packet)
}

func (m *mediaPipeline) refetchAligned(before *mediaPipeline) bool {
	if m.audioKind != baichuan.MediaAAC || !m.sharedClock {
		return true
	}
	audio := rescaleTimestamp(m.audioTS-before.audioTS, m.audioRate, 90000)
	video := m.videoTS.value - before.videoTS.value
	return max(video, audio)-min(video, audio) <= 90000
}
