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

type Profile struct {
	camera     *Camera
	key        ProfileKey
	generation uint64
	source     source
	open       profileOpener

	ctx         context.Context
	cancel      context.CancelFunc
	startCtx    context.Context
	startCancel context.CancelFunc
	ready       chan struct{}
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
	receivers []*core.Receiver
	video     *core.Receiver
	audio     *core.Receiver
	discovery discoverySnapshot

	talkMu        sync.Mutex
	talkKnown     bool
	talkSupported bool
	talkFormat    baichuan.TalkFormat

	recvBytes    atomic.Uint64
	videoFrames  atomic.Uint64
	audioSamples atomic.Uint64

	trackCheckInterval time.Duration
	trackStallTimeout  time.Duration
	trackRateWindow    time.Duration
}

func (p *Profile) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "reolink.Profile{Remote:%q, Channel:%d, Stream:%q, Generation:%d}",
		p.camera.remote, p.key.Channel, p.key.Stream, p.generation)
}

func newProfile(
	camera *Camera, key ProfileKey, generation uint64, s source, open profileOpener,
) *Profile {
	ctx, cancel := context.WithCancel(context.Background())
	startCtx, startCancel := context.WithCancel(ctx)
	return &Profile{
		camera: camera, key: key, generation: generation, source: s, open: open,
		ctx: ctx, cancel: cancel, startCtx: startCtx, startCancel: startCancel,
		ready: make(chan struct{}), done: make(chan struct{}), state: profileStarting,
		pipeline:           mediaPipeline{epoch: camera.nextEpoch(key, core.Now90000())},
		trackCheckInterval: profileTrackCheckInterval, trackStallTimeout: profileTrackStallTimeout,
		trackRateWindow: profileAudioRateWindow,
	}
}

func (p *Profile) run() {
	setup, cancel := context.WithTimeout(p.ctx, p.sourceTimeout())
	input, err := p.open(setup, p.camera, p.source)
	cancel()
	if err != nil {
		p.fail(fmt.Errorf("reolink: open profile: %w", err))
		close(p.done)
		return
	}
	p.mu.Lock()
	p.input = input
	p.discovery = input.Discovery()
	p.mu.Unlock()

	var probe mediaProbe
	if probe, err = p.probe(input); err == nil {
		p.initReceivers()
		err = p.markReady()
	}
	if err == nil {
		err = p.prepareStart(input, &probe)
	}
	probe.release()
	if err == nil {
		err = p.read(input)
	}
	failed := p.beginTerminal(err)
	var closeErr error
	if failed {
		closeErr = input.Abort()
	} else {
		closeErr = input.Close()
	}
	if failed && closeErr != nil {
		p.joinTerminal(closeErr)
	}
	p.finishSession(closeErr)
	close(p.done)
}

func (p *Profile) sourceTimeout() time.Duration {
	if p.source.config.Timeout > 0 {
		return p.source.config.Timeout
	}
	return baichuan.DefaultTimeout
}

func (p *Profile) probe(input profileInput) (mediaProbe, error) {
	ctx, cancel := context.WithTimeout(p.ctx, profileProbeTimeout)
	defer cancel()
	var probe mediaProbe
	for {
		videoReady := p.source.suppressVideo || p.pipeline.videoMedia != nil
		audioReady := p.source.suppressAudio || p.pipeline.audioMedia != nil
		if videoReady && audioReady {
			break
		}
		packet, err := input.Read(ctx)
		if err != nil {
			return mediaProbe{}, fmt.Errorf("reolink: probe media: %w", err)
		}
		p.record(packet)
		if p.retainPacket(packet) {
			if err = p.retainProbe(&probe, packet); err != nil {
				return mediaProbe{}, err
			}
		}
		switch packet.Kind {
		case baichuan.MediaVideoI:
			if !p.source.suppressVideo && p.pipeline.videoMedia == nil {
				if err = p.pipeline.probeVideo(packet); err != nil {
					return mediaProbe{}, err
				}
			}
		case baichuan.MediaAAC, baichuan.MediaADPCM:
			if !p.source.suppressAudio && p.pipeline.audioMedia == nil {
				if err = p.pipeline.configureAudio(packet); err != nil {
					return mediaProbe{}, err
				}
			}
		}
	}
	return probe, nil
}

func (p *Profile) initReceivers() {
	for _, media := range p.pipeline.medias {
		receiver := core.NewReceiver(media, media.Codecs[0])
		p.receivers = append(p.receivers, receiver)
		if media.Kind == core.KindVideo {
			p.video = receiver
		} else if media.Kind == core.KindAudio {
			p.audio = receiver
		}
	}
}

func (p *Profile) replay(probe mediaProbe) error {
	for _, packet := range probe.packets {
		frame, err := p.pipeline.frame(packet)
		if err != nil {
			return err
		}
		if frame.valid() {
			p.writeFrame(frame)
		}
	}
	return nil
}

func (p *Profile) read(input profileInput) error {
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
		p.record(packet)
		if !p.acceptPacket(packet) {
			continue
		}
		frame, err := p.pipeline.frame(packet)
		if err != nil {
			return err
		}
		if frame.valid() {
			p.writeFrame(frame)
		}
	}
}

func (p *Profile) writeFrame(frame mediaFrame) {
	if frame.track == trackVideo && p.video != nil {
		p.videoFrames.Add(1)
		p.video.WriteRTP(frame.packet)
	} else if frame.track == trackAudio && p.audio != nil {
		p.audioSamples.Add(uint64(frame.samples))
		p.audio.WriteRTP(frame.packet)
	}
}

func (p *Profile) prepareStart(input profileInput, probe *mediaProbe) error {
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
		p.record(packet)
		if p.retainPacket(packet) {
			err = p.retainProbe(probe, packet)
		}
		if err != nil {
			return err
		}
	}
}

func (p *Profile) retainProbe(probe *mediaProbe, packet baichuan.MediaPacket) error {
	if p.source.suppressVideo {
		probe.reset()
	}
	return probe.retain(packet)
}

func (p *Profile) acceptPacket(packet baichuan.MediaPacket) bool {
	switch packet.Kind {
	case baichuan.MediaVideoI, baichuan.MediaVideoP:
		return !p.source.suppressVideo
	case baichuan.MediaAAC, baichuan.MediaADPCM:
		return !p.source.suppressAudio
	default:
		return false
	}
}

func (p *Profile) retainPacket(packet baichuan.MediaPacket) bool {
	if !p.source.suppressVideo && !p.source.suppressAudio {
		return true
	}
	return p.acceptPacket(packet)
}

func (p *Profile) startProfile() {
	p.start.Do(p.startCancel)
}

func (p *Profile) closeReceivers() {
	p.close.Do(func() {
		for _, receiver := range p.receivers {
			receiver.Close()
		}
	})
}

func (p *Profile) record(packet baichuan.MediaPacket) {
	p.recvBytes.Add(uint64(len(packet.Data)))
}

func (p *Profile) markReady() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != profileStarting {
		return p.terminalError()
	}
	p.state = profileActive
	close(p.ready)
	return nil
}

func (p *Profile) waitReady() error {
	<-p.ready
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != profileActive {
		return p.terminalError()
	}
	return nil
}

func (p *Profile) terminalError() error {
	if p.terminal != nil {
		return p.terminal
	}
	return fmt.Errorf("reolink: profile %s", p.state)
}

func (p *Profile) beginTerminal(err error) bool {
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
	select {
	case <-p.ready:
	default:
		close(p.ready)
	}
	return true
}

func (p *Profile) joinTerminal(err error) {
	p.mu.Lock()
	p.terminal = errors.Join(p.terminal, err)
	p.mu.Unlock()
}

func (p *Profile) err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.terminalError()
}

func (p *Profile) closeError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeErr
}

func (p *Profile) finishSession(closeErr error) {
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
	}
	if epoch.video, epoch.videoSet = p.pipeline.videoTS.current(); epoch.videoSet || epoch.audioSet {
		p.camera.recordEpoch(p.key, epoch)
	}
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

func (p *Profile) fail(err error) {
	p.beginTerminal(err)
	p.finishSession(nil)
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
