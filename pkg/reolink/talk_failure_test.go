package reolink

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

func TestProducerTalkFailureKeepsPreview(t *testing.T) {
	for _, failure := range []string{"setup", "write", "idle"} {
		t.Run(failure, func(t *testing.T) {
			format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
			opener := &fakeProfileOpener{talk: &format}
			registry := testRegistry(opener.open)
			var logs bytes.Buffer
			registry.log = zerolog.New(&logs)
			producer, err := registry.Dial("reolink://sentinel-user:sentinel-pass@camera/main?backchannel=1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = producer.Stop() })
			input := opener.input(0)
			var attempted atomic.Bool
			var failing atomic.Bool
			failing.Store(true)
			input.startTalk = func(context.Context) (talkSession, error) {
				attempted.Store(true)
				if failing.Load() && failure == "setup" {
					return nil, errors.Join(&baichuan.StatusError{Command: 201, Code: 409}, errors.New("sentinel-camera-xml-private"))
				}
				return &fakeTalk{
					format: format,
					writeFn: func(context.Context, []byte) error {
						if failing.Load() && failure == "write" {
							return errors.New("sentinel-camera-xml-private write")
						}
						return nil
					},
					closeFn: func() error {
						if failing.Load() {
							return errors.New("sentinel-camera-xml-private stop")
						}
						return nil
					},
				}, nil
			}
			done := make(chan error, 1)
			video := producer.Receivers[0]
			packets := make(chan *rtp.Packet, 4)
			sender := core.NewSender(video.Media, video.Codec)
			sender.Handler = func(packet *rtp.Packet) { packets <- packet }
			sender.HandleRTP(video)
			defer sender.Close()
			go func() { done <- producer.Start() }()
			select {
			case <-packets:
			case <-time.After(time.Second):
				t.Fatal("initial video was not delivered")
			}
			codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}
			media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}}
			track := core.NewReceiver(media, codec)
			defer track.Close()
			if err = producer.AddTrack(media, codec, track); err != nil {
				t.Fatal(err)
			}
			track.WriteRTP(&rtp.Packet{Payload: []byte{0xaa, 0xaa}})
			waitFor(t, attempted.Load)
			// Taking the talk lock waits until the sender has finished the failed write.
			producer.talk.mu.Lock()
			producer.talk.idleAt = time.Now().Add(-time.Second)
			producer.talk.mu.Unlock()
			if failure == "idle" {
				producer.talk.closeIdle()
			}
			b := producer.talk
			b.mu.Lock()
			if b.talk != nil || len(b.samples) != 0 || b.encoder != (baichuan.ADPCMEncoder{}) {
				t.Error("failed talk retained session or encoder state")
			}
			if delay := time.Until(b.retryAt); delay < 4*time.Second || delay > talkRetryDelay {
				t.Errorf("retry delay = %v", delay)
			}
			b.mu.Unlock()
			attempts := input.starts.Load()
			for range 100 {
				if err = b.Write([]byte{0xaa}); err != nil {
					t.Fatal(err)
				}
			}
			if input.starts.Load() != attempts {
				t.Fatal("continuous audio retried during cooldown")
			}
			previous := producer.videoFrames.Load()
			previousAudio := producer.audioSamples.Load()
			input.results <- inputResult{packet: testH264PFrame(1_050_000)}
			input.results <- inputResult{packet: testAAC(1_064_000)}
			deadline := time.After(time.Second)
			for producer.videoFrames.Load() <= previous {
				select {
				case err = <-done:
					t.Fatalf("talk %s failure terminated preview: %v", failure, err)
				case <-deadline:
					t.Fatal("preview stopped delivering after talk failure")
				default:
					time.Sleep(time.Millisecond)
				}
			}
			if producer.stopCtx.Err() != nil || producer.ctx.Err() != nil || input.closed.Load() {
				t.Fatal("talk failure canceled or closed preview")
			}
			waitFor(t, func() bool { return producer.audioSamples.Load() > previousAudio })
			select {
			case <-packets:
			case <-time.After(time.Second):
				t.Fatal("receiver did not deliver video after talk failure")
			}
			b.mu.Lock()
			if bytes.Count(logs.Bytes(), []byte("reolink talkback failed")) != 1 ||
				bytes.Contains(logs.Bytes(), []byte("sentinel-user")) || bytes.Contains(logs.Bytes(), []byte("sentinel-pass")) ||
				bytes.Contains(logs.Bytes(), []byte("sentinel-camera-xml-private")) {
				t.Errorf("unexpected talk warnings: %s", logs.Bytes())
			}
			b.retryAt = time.Now().Add(-time.Second)
			b.mu.Unlock()
			if failure != "idle" {
				if err = b.Write([]byte{0xaa, 0xaa}); err == nil {
					t.Fatal("second failed attempt was suppressed after cooldown")
				}
				attempts++
				b.mu.Lock()
				if bytes.Count(logs.Bytes(), []byte("reolink talkback failed")) != 2 || time.Until(b.retryAt) < 4*time.Second {
					t.Error("second failure did not renew cooldown and warning")
				}
				b.retryAt = time.Now().Add(-time.Second)
				b.mu.Unlock()
			}
			failing.Store(false)
			// A codec replacement during recovery must retain the preview and allow new talk.
			codec = &core.Codec{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1}
			media.Codecs = []*core.Codec{codec}
			second := core.NewReceiver(media, codec)
			defer second.Close()
			if err = producer.AddTrack(media, codec, second); err != nil {
				t.Fatal(err)
			}
			if err = b.Write([]byte{0xd5, 0xd5}); err != nil {
				t.Fatal(err)
			}
			if input.starts.Load() != attempts+1 {
				t.Fatal("talk did not reopen after cooldown")
			}
			if err = producer.Stop(); err != nil {
				t.Fatal(err)
			}
			if err = producer.Stop(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("producer did not stop")
			}
		})
	}
}

func TestBackchannelNewSessionResetsTranscode(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "idle"
		if failure {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			format := baichuan.TalkFormat{SampleRate: 8000, SamplePrecision: 16, SamplesPerBlock: 4}
			opened := make(chan *fakeTalk, 2)
			b := &backchannel{producer: &Producer{stopCtx: ctx, stopCancel: cancel}, format: format}
			b.start = func(context.Context) (talkSession, error) {
				talk := &fakeTalk{format: format, blocks: make(chan []byte, 2)}
				opened <- talk
				return talk, nil
			}
			defer b.Close()
			if err := b.setCodec(&core.Codec{Name: core.CodecPCML, ClockRate: 16000, Channels: 1}); err != nil {
				t.Fatal(err)
			}
			loud := make([]byte, 2)
			binary.LittleEndian.PutUint16(loud, 10000)
			if err := b.Write(loud); err != nil {
				t.Fatal(err)
			}
			first := wantTalkOpen(t, opened)
			if failure {
				if err := b.Write([]byte{1}); err == nil {
					t.Fatal("malformed PCM did not fail")
				}
				b.mu.Lock()
				b.retryAt = time.Now().Add(-time.Second)
				b.mu.Unlock()
			} else {
				b.mu.Lock()
				b.idleAt = time.Now().Add(-time.Second)
				b.mu.Unlock()
				b.closeIdle()
			}
			if first.closes != 1 {
				t.Fatalf("old session close count: %d", first.closes)
			}
			// Audible source pair averages to zero, exposing old downsampler carry.
			fresh := make([]byte, 16)
			binary.LittleEndian.PutUint16(fresh, 100)
			binary.LittleEndian.PutUint16(fresh[2:], uint16(65536-100))
			if err := b.Write(fresh); err != nil {
				t.Fatal(err)
			}
			next := wantTalkOpen(t, opened)
			select {
			case block := <-next.blocks:
				samples, err := baichuan.DecodeADPCMBlock(block)
				if err != nil {
					t.Fatal(err)
				}
				for _, sample := range samples {
					if sample != 0 {
						t.Fatalf("previous session leaked into silent audio: %v", samples)
					}
				}
			case <-time.After(time.Second):
				t.Fatal("new session did not receive silent block")
			}
		})
	}
}
