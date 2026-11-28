package reolink

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const (
	talkIdleTimeout = 5 * time.Second
	talkRetryDelay  = 5 * time.Second
	talkSilenceTail = 2 * time.Second
	// Bound each packet independently of the camera's encoder block size.
	maxTalkPacketSamples = 4 * 8192
)

type talkSession interface {
	Format() baichuan.TalkFormat
	WriteBlock(context.Context, []byte) error
	Close(context.Context) error
}

type talkStarter func(context.Context) (talkSession, error)

type backchannel struct {
	producer *Producer
	format   baichuan.TalkFormat
	start    talkStarter

	mu          sync.Mutex
	closed      bool
	talk        talkSession
	encoder     baichuan.ADPCMEncoder
	codec       core.Codec
	transcode   func([]byte) []byte
	interpolate talkInterpolator
	lastSound   time.Time
	samples     []int16
	idle        *time.Timer
	idleAt      time.Time
	retryAt     time.Time
	sender      *core.Sender
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if p.talk == nil || media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly ||
		codec.Channels > 1 || !talkCodecSupported(codec) {
		return fmt.Errorf("reolink: unsupported talkback track")
	}

	p.talkMu.Lock()
	defer p.talkMu.Unlock()
	if p.stopCtx.Err() != nil {
		return p.stopCtx.Err()
	}
	if p.talk.sender != nil {
		p.talk.sender.Close()
		p.talk.sender.Wait()
	}
	if err := p.talk.setCodec(codec); err != nil {
		return err
	}
	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		_ = p.talk.Write(packet.Payload) // The backchannel owns failure cleanup and reporting.
	}
	sender.HandleRTP(track)
	p.talk.sender = sender
	p.Senders = []*core.Sender{sender}
	return nil
}

func talkCodecs() []*core.Codec {
	codecs := []*core.Codec{
		{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1, PayloadType: 8},
		{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1, PayloadType: 0},
	}
	for _, codec := range pcm.ProducerCodecs() {
		if codec.Name != core.CodecPCM && codec.Name != core.CodecPCML ||
			codec.ClockRate != 8000 && codec.ClockRate != 16000 {
			continue
		}
		codec.Channels = 1
		codec.PayloadType = core.PayloadTypeRAW
		codecs = append(codecs, codec)
	}
	return codecs
}

func talkCodecSupported(codec *core.Codec) bool {
	if codec.ClockRate == 0 {
		return false
	}
	for _, candidate := range talkCodecs() {
		if candidate.Name == codec.Name && candidate.ClockRate == codec.ClockRate {
			return true
		}
	}
	return false
}

func (b *backchannel) setCodec(codec *core.Codec) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("reolink: talkback closed")
	}
	b.codec = *codec
	b.reset()
	return nil
}

func (b *backchannel) reset() {
	target := &core.Codec{Name: core.CodecPCML, ClockRate: b.format.SampleRate, Channels: 1}
	if b.codec.ClockRate == 8000 && b.format.SampleRate == 16000 {
		target.ClockRate = 8000
	}
	b.transcode = pcm.Transcode(target, &b.codec)
	b.interpolate = talkInterpolator{}
	b.lastSound = time.Time{}
	b.encoder = baichuan.ADPCMEncoder{}
	b.samples = b.samples[:0]
}

func (b *backchannel) Write(payload []byte) (err error) {
	b.mu.Lock()
	defer func() {
		if err != nil && !b.closed && b.producer.stopCtx.Err() == nil {
			b.failed(err)
		}
		b.mu.Unlock()
	}()
	if b.closed {
		return fmt.Errorf("reolink: talkback closed")
	}
	if !b.retryAt.IsZero() {
		if time.Now().Before(b.retryAt) {
			return nil
		}
		b.retryAt = time.Time{}
	}
	inputSamples := uint64(len(payload))
	if b.codec.Name == core.CodecPCM || b.codec.Name == core.CodecPCML {
		if len(payload)&1 != 0 {
			return fmt.Errorf("reolink: talkback PCM payload has odd length %d", len(payload))
		}
		inputSamples /= 2
	}
	if inputSamples > maxTalkPacketSamples*uint64(b.codec.ClockRate)/uint64(b.format.SampleRate) {
		return fmt.Errorf("reolink: talkback packet exceeds sample limit")
	}
	quiet := talkQuiet(payload, b.codec.Name)
	if quiet && (b.talk == nil || time.Since(b.lastSound) >= talkSilenceTail) {
		return nil
	}
	if err := b.open(); err != nil {
		return err
	}
	if !quiet {
		b.lastSound = time.Now()
	}
	pcmBytes := b.transcode(payload)
	if len(pcmBytes)&1 != 0 {
		return fmt.Errorf("reolink: talkback produced odd PCM length %d", len(pcmBytes))
	}
	if b.codec.ClockRate == 8000 && b.format.SampleRate == 16000 {
		pcmBytes = b.interpolate.convert(pcmBytes)
	}
	blockSize := int(b.format.SamplesPerBlock)
	if cap(b.samples) < blockSize {
		b.samples = make([]int16, 0, blockSize)
	}
	for i := 0; i < len(pcmBytes); i += 2 {
		b.samples = append(b.samples, int16(binary.LittleEndian.Uint16(pcmBytes[i:])))
		if len(b.samples) < blockSize {
			continue
		}
		block, err := b.encoder.EncodeBlock(b.samples[:blockSize])
		b.samples = b.samples[:0]
		if err != nil {
			return err
		}
		if err = b.talk.WriteBlock(b.producer.stopCtx, block); err != nil {
			return err
		}
		b.producer.addSend(len(block))
	}
	b.idleAt = time.Now().Add(talkIdleTimeout)
	if b.idle == nil {
		b.idle = time.AfterFunc(talkIdleTimeout, b.closeIdle)
	} else {
		b.idle.Reset(talkIdleTimeout)
	}
	return nil
}

func (b *backchannel) open() error {
	if b.talk != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(b.producer.stopCtx, baichuan.DefaultTimeout)
	start := b.start
	if start == nil {
		start = b.producer.startTalk
	}
	talk, err := start(ctx)
	if err == nil {
		if format := talk.Format(); format != b.format {
			err = fmt.Errorf("reolink: talkback format changed from %+v to %+v", b.format, format)
		} else {
			b.talk = talk
			b.reset()
		}
	}
	cancel()
	if err != nil {
		var cleanup error
		if talk != nil {
			closeCtx, closeCancel := context.WithTimeout(b.producer.stopCtx, baichuan.DefaultTimeout)
			cleanup = errors.Join(cleanup, talk.Close(closeCtx))
			closeCancel()
		}
		return fmt.Errorf("reolink: start talkback: %w", errors.Join(err, cleanup))
	}
	return nil
}

func (b *backchannel) closeIdle() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if wait := time.Until(b.idleAt); wait > 0 {
		b.idle.Reset(wait)
		b.mu.Unlock()
		return
	}
	err := b.closeSession(b.producer.stopCtx)
	if err != nil && b.producer.stopCtx.Err() == nil {
		b.failed(fmt.Errorf("reolink: stop idle talkback: %w", err))
	}
	b.mu.Unlock()
}

// failed runs with mu held. A failed talk session does not own the preview lifetime.
func (b *backchannel) failed(err error) {
	err = errors.Join(err, b.closeSession(b.producer.stopCtx))
	b.reset()
	b.retryAt = time.Now().Add(talkRetryDelay)
	if p := b.producer; p.camera != nil {
		p.camera.registry.log.Warn().Str("error_class", errorClass(err)).Str("camera", p.camera.remote).
			Uint8("channel", p.key.Channel).Msg("reolink talkback failed")
	}
}

func (b *backchannel) closeSession(ctx context.Context) error {
	if b.talk == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, baichuan.DefaultTimeout)
	err := b.talk.Close(ctx)
	cancel()
	b.talk = nil
	b.reset()
	return err
}

func (b *backchannel) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	if b.idle != nil {
		b.idle.Stop()
	}
	b.mu.Unlock()

	if b.sender != nil {
		b.sender.Close()
		b.sender.Wait()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	timeout := baichuan.DefaultTimeout
	if b.producer.stopCtx.Err() != nil {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(b.producer.stopCtx), timeout)
	defer cancel()
	return b.closeSession(ctx)
}
