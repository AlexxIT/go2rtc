package reolink

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type fakeTalk struct {
	format   baichuan.TalkFormat
	blocks   chan []byte
	writeErr error
	writeFn  func(context.Context, []byte) error
	closeFn  func() error
	closes   int
}

func (t *fakeTalk) Format() baichuan.TalkFormat { return t.format }
func (t *fakeTalk) WriteBlock(ctx context.Context, block []byte) error {
	if t.writeFn != nil {
		return t.writeFn(ctx, block)
	}
	if t.writeErr != nil {
		return t.writeErr
	}
	t.blocks <- append([]byte(nil), block...)
	return nil
}
func (t *fakeTalk) Close(context.Context) error {
	t.closes++
	if t.closeFn != nil {
		return t.closeFn()
	}
	return nil
}

func TestBackchannelAddTrackReplacementAndIdleReopen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Producer{ctx: ctx, cancel: cancel}
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	opened := make(chan *fakeTalk, 2)
	b := &backchannel{producer: p, format: format}
	b.start = func(context.Context) (talkSession, error) {
		talk := &fakeTalk{format: format, blocks: make(chan []byte, 1)}
		opened <- talk
		return talk, nil
	}
	p.talk = b

	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}}
	first := core.NewReceiver(media, codec)
	if err := p.AddTrack(media, codec, first); err != nil {
		t.Fatal(err)
	}
	first.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	firstTalk := wantTalkOpen(t, opened)
	wantBlock(t, firstTalk, format.BytesPerBlock())

	second := core.NewReceiver(media, codec)
	if err := p.AddTrack(media, codec, second); err != nil {
		t.Fatal(err)
	}
	second.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	wantBlock(t, firstTalk, format.BytesPerBlock())
	select {
	case <-opened:
		t.Fatal("sender replacement reopened talk session")
	default:
	}

	b.mu.Lock()
	b.idleAt = time.Now().Add(-time.Second)
	b.mu.Unlock()
	b.closeIdle()
	if firstTalk.closes != 1 {
		t.Fatalf("idle close count: %d", firstTalk.closes)
	}
	second.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	secondTalk := wantTalkOpen(t, opened)
	wantBlock(t, secondTalk, format.BytesPerBlock())

	cancel()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second.Close()
	if secondTalk.closes != 1 {
		t.Fatalf("close count for reopened session: %d", secondTalk.closes)
	}
}

func TestBackchannelWriteFailureCancelsProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Producer{ctx: ctx, cancel: cancel}
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	b := &backchannel{producer: p, format: format}
	b.start = func(context.Context) (talkSession, error) {
		return &fakeTalk{
			format: format, blocks: make(chan []byte, 1), writeErr: errors.New("sentinel write"),
		}, nil
	}
	p.talk = b
	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}}
	track := core.NewReceiver(media, codec)
	if err := p.AddTrack(media, codec, track); err != nil {
		t.Fatal(err)
	}
	track.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	select {
	case <-p.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("talk write failure did not cancel producer")
	}
	if err := p.terminalError(context.Canceled); err == nil || !strings.Contains(err.Error(), "sentinel write") {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	track.Close()
}

func TestBackchannelCloseUnblocksActiveWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Producer{ctx: ctx, cancel: cancel}
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	started := make(chan struct{})
	talk := &fakeTalk{format: format, writeFn: func(ctx context.Context, _ []byte) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	b := &backchannel{producer: p, format: format, talk: talk}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	wrote := make(chan error, 1)
	go func() { wrote <- b.Write([]byte{0xd5, 0xd5}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("talk write did not start")
	}
	cancel()
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backchannel close did not unblock write")
	}
	select {
	case err := <-wrote:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected write result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("talk write did not return")
	}
	if talk.closes != 1 {
		t.Fatalf("talk close count: %d", talk.closes)
	}
}

func TestBackchannelStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Producer{ctx: ctx, cancel: cancel}
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	b := &backchannel{producer: p, format: format}
	b.start = func(context.Context) (talkSession, error) {
		return nil, errors.New("sentinel start")
	}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if err := b.Write([]byte{0xd5, 0xd5}); err == nil || !strings.Contains(err.Error(), "sentinel start") {
		t.Fatalf("unexpected start error: %v", err)
	}
}

func TestBackchannelLinearPCM(t *testing.T) {
	for _, codec := range talkCodecs() {
		if codec.Name != core.CodecPCM && codec.Name != core.CodecPCML {
			continue
		}
		t.Run(codec.String(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
			talk := &fakeTalk{format: format, blocks: make(chan []byte, 1)}
			b := &backchannel{
				producer: &Producer{ctx: ctx, cancel: cancel}, format: format, talk: talk,
			}
			if err := b.setCodec(codec); err != nil {
				t.Fatal(err)
			}
			inputSamples := (uint64(format.SamplesPerBlock)*uint64(codec.ClockRate) +
				uint64(format.SampleRate) - 1) / uint64(format.SampleRate)
			if err := b.Write(make([]byte, int(inputSamples)*2)); err != nil {
				t.Fatal(err)
			}
			wantBlock(t, talk, format.BytesPerBlock())
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			if talk.closes != 1 {
				t.Fatalf("session close count: %d", talk.closes)
			}
		})
	}
}

func TestTalkCodecSupport(t *testing.T) {
	tests := []struct {
		name string
		rate uint32
	}{
		{core.CodecPCMA, 8000}, {core.CodecPCMU, 8000},
		{core.CodecPCML, 8000}, {core.CodecPCM, 8000},
		{core.CodecPCML, 16000}, {core.CodecPCM, 16000},
	}
	for _, test := range tests {
		codec := &core.Codec{Name: test.name, ClockRate: test.rate, Channels: 1}
		if !talkCodecSupported(codec) {
			t.Fatalf("unsupported parent PCM codec %s", codec)
		}
	}
	if talkCodecSupported(&core.Codec{Name: core.CodecPCML, ClockRate: 44100, Channels: 1}) {
		t.Fatal("accepted unadvertised PCM clock rate")
	}
}

func TestBackchannelRejectsInvalidPCM(t *testing.T) {
	b := &backchannel{
		producer: &Producer{ctx: context.Background()},
		format:   baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 4},
	}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCML, ClockRate: 16000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if err := b.Write([]byte{0}); err == nil || !strings.Contains(err.Error(), "odd length") {
		t.Fatalf("unexpected odd PCM result: %v", err)
	}
	if talkCodecSupported(&core.Codec{Name: core.CodecPCML}) {
		t.Fatal("accepted PCM without a clock rate")
	}
}

func wantTalkOpen(t *testing.T, opened <-chan *fakeTalk) *fakeTalk {
	t.Helper()
	select {
	case talk := <-opened:
		return talk
	case <-time.After(time.Second):
		t.Fatal("talk session was not opened")
		return nil
	}
}

func wantBlock(t *testing.T, talk *fakeTalk, size int) {
	t.Helper()
	select {
	case block := <-talk.blocks:
		if len(block) != size {
			t.Fatalf("unexpected ADPCM block size: %d", len(block))
		}
	case <-time.After(time.Second):
		t.Fatal("talk block was not written")
	}
}
