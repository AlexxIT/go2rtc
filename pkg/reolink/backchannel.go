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

const talkIdleTimeout = 5 * time.Second

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

	mu        sync.Mutex
	closed    bool
	talk      talkSession
	encoder   baichuan.ADPCMEncoder
	codec     core.Codec
	transcode func([]byte) []byte
	samples   []int16
	idle      *time.Timer
	idleAt    time.Time
	sender    *core.Sender
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if p.talk == nil || media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly ||
		codec.Channels > 1 || !talkCodecSupported(codec) {
		return fmt.Errorf("reolink: unsupported talkback track")
	}

	p.talkMu.Lock()
	defer p.talkMu.Unlock()
	if p.ctx.Err() != nil {
		return p.ctx.Err()
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
		if err := p.talk.Write(packet.Payload); err != nil && p.ctx.Err() == nil {
			p.fail(fmt.Errorf("reolink: write talkback: %w", err))
		}
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
	target := &core.Codec{Name: core.CodecPCML, ClockRate: b.format.SampleRate, Channels: 1}
	b.transcode = pcm.Transcode(target, &b.codec)
	b.samples = b.samples[:0]
	return nil
}

func (b *backchannel) Write(payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("reolink: talkback closed")
	}
	blockSamples := uint64(b.format.SamplesPerBlock)
	inputSamples := uint64(len(payload))
	if b.codec.Name == core.CodecPCM || b.codec.Name == core.CodecPCML {
		if len(payload)&1 != 0 {
			return fmt.Errorf("reolink: talkback PCM payload has odd length %d", len(payload))
		}
		inputSamples /= 2
	}
	expanded := (inputSamples*uint64(b.format.SampleRate) + uint64(b.codec.ClockRate) - 1) /
		uint64(b.codec.ClockRate)
	if expanded > blockSamples*4 {
		return fmt.Errorf("reolink: talkback packet exceeds sample limit")
	}
	if err := b.open(); err != nil {
		return err
	}
	pcmBytes := b.transcode(payload)
	if len(pcmBytes)&1 != 0 {
		return fmt.Errorf("reolink: talkback produced odd PCM length %d", len(pcmBytes))
	}
	blockSize := int(blockSamples)
	for i := 0; i < len(pcmBytes); i += 2 {
		b.samples = append(b.samples, int16(binary.LittleEndian.Uint16(pcmBytes[i:])))
	}
	for len(b.samples) >= blockSize {
		block, err := b.encoder.EncodeBlock(b.samples[:blockSize])
		if err != nil {
			return err
		}
		if err = b.talk.WriteBlock(b.producer.ctx, block); err != nil {
			return err
		}
		b.producer.addSend(len(block))
		if len(b.samples) == blockSize {
			b.samples = b.samples[:0]
		} else {
			b.samples = b.samples[blockSize:]
		}
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
	ctx, cancel := context.WithTimeout(b.producer.ctx, baichuan.DefaultTimeout)
	start := b.start
	if start == nil {
		start = b.producer.profile.startTalk
	}
	talk, err := start(ctx)
	if err == nil {
		if format := talk.Format(); format != b.format {
			err = fmt.Errorf("reolink: talkback format changed from %+v to %+v", b.format, format)
		} else {
			b.talk = talk
			b.encoder = baichuan.ADPCMEncoder{}
		}
	}
	cancel()
	if err != nil {
		var cleanup error
		if talk != nil {
			closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(b.producer.ctx), baichuan.DefaultTimeout)
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
	err := b.closeSession()
	b.mu.Unlock()
	if err != nil && b.producer.ctx.Err() == nil {
		b.producer.fail(fmt.Errorf("reolink: stop idle talkback: %w", err))
	}
}

func (b *backchannel) closeSession() error {
	if b.talk == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(b.producer.ctx), baichuan.DefaultTimeout)
	err := b.talk.Close(ctx)
	cancel()
	b.talk = nil
	b.samples = b.samples[:0]
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
	return b.closeSession()
}
