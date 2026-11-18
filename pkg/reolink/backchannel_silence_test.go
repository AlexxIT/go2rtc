package reolink

import (
	"context"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestTalkQuiet(t *testing.T) {
	for _, tc := range []struct {
		codec   string
		payload []byte
		quiet   bool
	}{
		{core.CodecPCMA, []byte{0xd5, 0x55}, true},                         // +8, -8
		{core.CodecPCMA, []byte{0xd4}, false},                              // +24
		{core.CodecPCMA, []byte{0x54}, false},                              // -24
		{core.CodecPCMU, []byte{0xff, 0x7f, 0xfe, 0x7e, 0xfd, 0x7d}, true}, // +/-0,8,16
		{core.CodecPCMU, []byte{0xfc}, false},                              // +24
		{core.CodecPCMU, []byte{0x7c}, false},                              // -24
		{core.CodecPCML, []byte{0x10, 0x00, 0xf0, 0xff}, true},             // +16, -16
		{core.CodecPCML, []byte{0x11, 0x00}, false},                        // +17
		{core.CodecPCML, []byte{0xef, 0xff}, false},                        // -17
		{core.CodecPCML, []byte{0x00, 0x10}, false},                        // 4096
		{core.CodecPCM, []byte{0x00, 0x10, 0xff, 0xf0}, true},              // +16, -16
		{core.CodecPCM, []byte{0x00, 0x11}, false},                         // +17
		{core.CodecPCM, []byte{0xff, 0xef}, false},                         // -17
		{core.CodecPCM, []byte{0x10, 0x00}, false},                         // 4096
	} {
		if got := talkQuiet(tc.payload, tc.codec); got != tc.quiet {
			t.Errorf("%s %x quiet = %v, want %v", tc.codec, tc.payload, got, tc.quiet)
		}
	}
}

func TestBackchannelInitialSilence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		quiet []byte
	}{
		{core.CodecPCMA, []byte{0xd5, 0xd5}},
		{core.CodecPCMU, []byte{0xff, 0x7f}},
		{core.CodecPCML, []byte{0, 0, 16, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opens := 0
			b := &backchannel{producer: &Producer{stopCtx: context.Background()}, format: baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 4}}
			defer b.Close()
			b.start = func(context.Context) (talkSession, error) {
				opens++
				return &fakeTalk{format: b.format, writeFn: func(context.Context, []byte) error { return nil }}, nil
			}
			if err := b.setCodec(&core.Codec{Name: tc.name, ClockRate: 8000, Channels: 1}); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if err := b.Write(tc.quiet); err != nil {
					t.Fatal(err)
				}
			}
			if opens != 0 || b.idle != nil {
				t.Fatalf("quiet input opened %d sessions, timer %v", opens, b.idle)
			}
			b.retryAt = time.Now().Add(time.Minute)
			if err := b.Write([]byte{0x41, 0x41}); err != nil {
				t.Fatal(err)
			}
			if opens != 0 {
				t.Fatal("pending retry bypassed")
			}
		})
	}
}

func TestBackchannelSilenceRelease(t *testing.T) {
	format := baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 4}
	blocks, opens, closes := 0, 0, 0
	b := &backchannel{producer: &Producer{stopCtx: context.Background()}, format: format}
	defer b.Close()
	b.start = func(context.Context) (talkSession, error) {
		opens++
		return &fakeTalk{format: format,
			writeFn: func(context.Context, []byte) error { blocks++; return nil },
			closeFn: func() error { closes++; return nil }}, nil
	}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	write := func(p []byte) {
		t.Helper()
		if err := b.Write(p); err != nil {
			t.Fatal(err)
		}
	}
	write([]byte{0xaa, 0xaa})
	if b.lastSound.IsZero() {
		t.Fatal("open reset lost sound timestamp")
	}
	write([]byte{0xd5, 0xd5})
	if blocks != 2 {
		t.Fatalf("short pause blocks = %d", blocks)
	}
	b.mu.Lock()
	b.lastSound = time.Now().Add(-talkSilenceTail - time.Millisecond)
	deadline := b.idleAt
	b.mu.Unlock()
	for range 3 {
		write([]byte{0xd5, 0xd5})
	}
	if blocks != 2 || b.idleAt != deadline {
		t.Fatal("long mute fed audio or renewed idle")
	}
	b.mu.Lock()
	b.idleAt = time.Now().Add(-time.Millisecond)
	b.mu.Unlock()
	b.closeIdle()
	if b.interpolate != (talkInterpolator{}) {
		t.Fatal("idle close retained interpolation history")
	}
	write([]byte{0xd5, 0xd5})
	if opens != 1 || closes != 1 {
		t.Fatalf("muted reopen opens=%d closes=%d", opens, closes)
	}
	write([]byte{0xaa, 0xaa})
	if opens != 2 || blocks != 3 {
		t.Fatal("sound did not reopen session")
	}
	if b.interpolate.history[0] == 0 {
		t.Fatal("active interpolation lost history")
	}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if b.interpolate != (talkInterpolator{}) {
		t.Fatal("codec change retained interpolation history")
	}
	write([]byte{0x41, 0x41})
	if err := b.setCodec(&core.Codec{Name: core.CodecPCML, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	write([]byte{0x41, 0x41, 0x41, 0x41})
	if err := b.Write([]byte{1}); err == nil {
		t.Fatal("invalid PCM accepted")
	}
	if b.interpolate != (talkInterpolator{}) {
		t.Fatal("failure retained interpolation history")
	}
	before := opens
	write([]byte{0x41, 0x41, 0x41, 0x41})
	if opens != before {
		t.Fatal("sound bypassed failure retry")
	}
}
