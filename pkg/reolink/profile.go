package reolink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	profileProbeTimeout       = 10 * time.Second
	profileTrackCheckInterval = time.Second
	profileTrackStallTimeout  = 15 * time.Second
	profileAudioRateWindow    = 60 * time.Second
	profileRetryDelay         = time.Second
)

type profileState uint8

const (
	profileStarting profileState = iota
	profileActive
	profileClosing
	profileFailed
	profileClosed
)

func (s profileState) String() string {
	switch s {
	case profileStarting:
		return "starting"
	case profileActive:
		return "active"
	case profileClosing:
		return "closing"
	case profileFailed:
		return "failed"
	case profileClosed:
		return "closed"
	default:
		return "unknown"
	}
}

type Producer struct {
	core.Connection
	producerFormat

	camera       *Camera
	key          ProfileKey
	generation   uint64
	attemptBegan time.Time
	source       *source
	open         profileOpener

	// Stop intent remains separate from terminal input failure.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	stop       sync.Once
	stopErr    error
	send       atomic.Uint64
	talkMu     sync.Mutex
	talk       *backchannel

	ctx         context.Context
	cancel      context.CancelFunc
	startCtx    context.Context
	startCancel context.CancelFunc
	done        chan struct{}
	start       sync.Once
	close       sync.Once

	mu        sync.Mutex
	state     profileState
	released  bool
	input     profileInput
	terminal  error
	closeErr  error
	errClass  string
	pipeline  mediaPipeline
	video     *core.Receiver
	audio     *core.Receiver
	discovery discoverySnapshot
	history   *refetchHistory

	probeTalkMu   sync.Mutex
	talkKnown     bool
	talkSupported bool
	talkFormat    baichuan.TalkFormat

	recvBytes      atomic.Uint64
	videoFrames    atomic.Uint64
	audioSamples   atomic.Uint64
	videoRate      atomic.Uint64
	videoRateEpoch uint32 // Odd until an accepted recovery frame anchors the video rate.
	videoResyncs   atomic.Uint64

	trackCheckInterval time.Duration
	trackStallTimeout  time.Duration
	trackRateWindow    time.Duration
}

func newProducer(
	camera *Camera, key ProfileKey, generation uint64, s source, open profileOpener,
) *Producer {
	ctx, cancel := context.WithCancel(context.Background())
	startCtx, startCancel := context.WithCancel(ctx)
	epoch := camera.nextEpoch(key, nowTimestamp(90000), !s.suppressAudio)
	sharedClock := !s.suppressVideo && !s.suppressAudio
	if sharedClock && epoch.audioSet {
		epoch.video = max(epoch.video, rescaleTimestampCeil(epoch.audio, epoch.audioRate, 90000))
	}
	p := &Producer{
		camera: camera, key: key, generation: generation, attemptBegan: time.Now(),
		source: &s, open: open,
		ctx: ctx, cancel: cancel, startCtx: startCtx, startCancel: startCancel,
		done: make(chan struct{}), state: profileStarting,
		pipeline: mediaPipeline{epoch: epoch, sharedClock: sharedClock,
			videoOnly: s.suppressAudio && !s.suppressVideo},
		trackCheckInterval: profileTrackCheckInterval, trackStallTimeout: profileTrackStallTimeout,
		trackRateWindow: profileAudioRateWindow,
	}
	p.producerFormat = producerFormat{connection: &p.Connection, key: key, generation: generation}
	p.pipeline.videoResyncs = &p.videoResyncs
	return p
}

func (p *Producer) setup() error {
	setup, cancel := context.WithTimeout(p.ctx, p.sourceTimeout())
	input, err := p.open(setup, p.camera, *p.source)
	cancel()
	if err != nil {
		err = fmt.Errorf("reolink: open profile: %w", err)
		p.finish(nil, err)
		return p.err()
	}
	p.mu.Lock()
	p.input = input
	p.discovery = input.Discovery()
	p.mu.Unlock()

	probe, err := p.probe(input)
	if err != nil {
		probe.release()
		p.finish(input, err)
		return p.err()
	}
	p.initReceivers()
	p.mu.Lock()
	if p.source.refetch && (p.audio == nil || p.pipeline.audioKind == baichuan.MediaAAC) {
		p.history = new(refetchHistory)
	}
	p.state = profileActive
	p.mu.Unlock()
	// The worker owns input teardown and session publication from this point.
	go p.run(input, probe)
	return nil
}

func (p *Producer) run(input profileInput, probe mediaProbe) {
	err := p.prepareStart(input, &probe)
	probe.release()
	var lastRefetch time.Time
	for err == nil {
		err = p.read(input)
		if time.Since(lastRefetch) < time.Minute || !p.canRefetch(err) {
			break
		}
		lastRefetch = time.Now()
		input, err = p.refetch(input, err)
	}
	p.finish(input, err)
}

func (p *Producer) finish(input profileInput, err error) {
	failed := p.beginTerminal(err)
	var closeErr error
	if input != nil {
		if failed {
			closeErr = input.Abort()
		} else {
			closeErr = input.Close()
		}
	}
	if failed && closeErr != nil {
		p.joinTerminal(closeErr)
	}
	p.finishSession(closeErr)
	close(p.done)
}

func (p *Producer) sourceTimeout() time.Duration {
	if p.source.config.Timeout > 0 {
		return p.source.config.Timeout
	}
	return baichuan.DefaultTimeout
}

func (p *Producer) initReceivers() {
	p.Medias = p.pipeline.medias
	for _, media := range p.pipeline.medias {
		receiver := core.NewReceiver(media, media.Codecs[0])
		p.Receivers = append(p.Receivers, receiver)
		if media.Kind == core.KindVideo {
			p.video = receiver
		} else if media.Kind == core.KindAudio {
			p.audio = receiver
		}
	}
}

func (p *Producer) replay(probe mediaProbe) error {
	for _, packet := range probe.packets {
		if err := p.writePacket(packet); err != nil {
			return err
		}
	}
	return nil
}

func (p *Producer) read(input profileInput) error {
	ctx, cancel := context.WithCancel(p.ctx)
	stalled := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		p.watchTracks(ctx, cancel, stalled)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	for {
		packet, err := input.Read(ctx)
		if err != nil {
			select {
			case err = <-stalled:
				return err
			default:
			}
			return fmt.Errorf("reolink: read media: %w", err)
		}
		if packet.Kind == baichuan.MediaDiscontinuity {
			p.noteDiscontinuity(nil)
			continue
		}
		p.record(packet)
		if !p.acceptPacket(packet) {
			continue
		}
		if err := p.writePacket(packet); err != nil {
			return err
		}
	}
}

func (p *Producer) writeFrame(frame mediaFrame) {
	if p.history != nil {
		p.history.record(frame, p.pipeline.videoTS.last, p.pipeline.audioKind)
	}
	if frame.track == trackVideo && p.video != nil {
		if p.audio != nil {
			if p.videoRateEpoch&1 != 0 {
				p.videoRateEpoch++
			}
			p.videoRate.Store(uint64(p.videoRateEpoch)<<32 | uint64(frame.packet.Timestamp))
		}
		p.videoFrames.Add(1)
		p.video.WriteRTP(frame.packet)
	} else if frame.track == trackAudio && p.audio != nil {
		p.audioSamples.Add(uint64(frame.samples))
		p.audio.WriteRTP(frame.packet)
	}
}

func (p *Producer) prepareStart(input profileInput, probe *mediaProbe) error {
	for {
		select {
		case <-p.startCtx.Done():
			if err := p.ctx.Err(); err != nil {
				return err
			}
			return p.replay(*probe)
		default:
		}
		packet, err := input.Read(p.startCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) && p.startCtx.Err() != nil && p.ctx.Err() == nil {
				return p.replay(*probe)
			}
			return fmt.Errorf("reolink: wait for start: %w", err)
		}
		if packet.Kind == baichuan.MediaDiscontinuity {
			p.noteDiscontinuity(probe)
			continue
		}
		p.record(packet)
		if p.retainPacket(packet) {
			err = p.retainProbe(probe, packet)
		}
		if err != nil {
			return err
		}
	}
}

func (p *Producer) noteDiscontinuity(probe *mediaProbe) {
	if probe != nil {
		probe.reset()
	}
	if !p.source.suppressVideo {
		p.pipeline.suppressInvalidVideo()
		p.invalidateVideoRate()
	}
}

func (p *Producer) retainProbe(probe *mediaProbe, packet baichuan.MediaPacket) error {
	if p.source.suppressVideo {
		probe.reset()
	}
	return probe.retain(packet)
}

func (p *Producer) acceptPacket(packet baichuan.MediaPacket) bool {
	switch packet.Kind {
	case baichuan.MediaVideoI, baichuan.MediaVideoP:
		return !p.source.suppressVideo
	case baichuan.MediaAAC, baichuan.MediaADPCM:
		return !p.source.suppressAudio
	default:
		return false
	}
}

func (p *Producer) retainPacket(packet baichuan.MediaPacket) bool {
	if !p.source.suppressVideo && !p.source.suppressAudio {
		return true
	}
	return p.acceptPacket(packet)
}

func (p *Producer) closeReceivers() {
	p.close.Do(func() {
		for _, receiver := range p.Receivers {
			receiver.Close()
		}
	})
}

func (p *Producer) record(packet baichuan.MediaPacket) {
	p.recvBytes.Add(uint64(len(packet.Data)))
}

func (p *Producer) activeError() error {
	p.mu.Lock()
	input := p.input
	p.mu.Unlock()
	var err error
	if input != nil {
		err = input.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.terminal != nil {
		return p.terminal
	}
	if ctxErr := p.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if p.state != profileActive || input == nil {
		return p.terminalError()
	}
	return err
}

func (p *Producer) terminalError() error {
	if p.terminal != nil {
		return p.terminal
	}
	return fmt.Errorf("reolink: profile %s", p.state)
}

func (p *Producer) beginTerminal(err error) bool {
	p.camera.registry.mu.Lock()
	defer p.camera.registry.mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == profileClosing || p.state == profileClosed || p.state == profileFailed {
		return false
	}
	if _, ok := p.camera.profiles[p]; ok {
		delete(p.camera.profiles, p)
	}
	p.state = profileFailed
	p.terminal = err
	p.errClass = errorClass(err)
	p.cancel()
	return true
}

func (p *Producer) joinTerminal(err error) {
	p.mu.Lock()
	p.terminal = errors.Join(p.terminal, err)
	p.mu.Unlock()
}

func (p *Producer) err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.terminalError()
}

func (p *Producer) closeError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeErr
}

func (p *Producer) finishSession(closeErr error) {
	registry := p.camera.registry
	registry.mu.Lock()
	p.mu.Lock()
	p.closeErr = closeErr
	if p.state == profileClosing {
		p.state = profileClosed
		if closeErr != nil {
			p.terminal = closeErr
			p.errClass = errorClass(closeErr)
		}
	}
	state := p.state.String()
	errClass := p.errClass
	epoch := mediaEpoch{
		audio: p.pipeline.audioTS, audioRate: p.pipeline.audioRate, audioSet: p.pipeline.audioMedia != nil,
		aac: aacSequence{next: p.pipeline.audioSeq, config: p.pipeline.audioConfig,
			valid: p.pipeline.audioKind == baichuan.MediaAAC},
	}
	epoch.video, epoch.videoSet = p.pipeline.videoTS.current()
	p.camera.recordEpoch(p.key, epoch, !p.source.suppressAudio, p.audioSamples.Load() != 0)
	p.mu.Unlock()
	p.camera.removeSession(p.key.Stream)
	registry.removeCameraLocked(p.camera)
	registry.mu.Unlock()
	event := registry.log.Debug().Str("camera", p.camera.remote).Uint8("channel", p.key.Channel).
		Str("profile", string(p.key.Stream)).Uint64("generation", p.generation).
		Str("state", state).Str("error_class", errClass)
	if closeErr != nil {
		event = event.Err(closeErr)
	}
	event.Msg("reolink profile stopped")
}

func errorClass(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "eof"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "network"
	}
	var mediaErr *mediaError
	if errors.As(err, &mediaErr) {
		return "media"
	}
	return "protocol"
}
