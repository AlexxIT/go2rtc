package reolink

import (
	"context"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestBackchannelPacketSize(t *testing.T) {
	tests := []struct {
		name     string
		codec    string
		rate     uint32
		camera   uint32
		block    uint32
		samples  []int
		blocks   []int
		leftover int
	}{
		{"20ms_small_block", core.CodecPCMA, 8000, 16000, 64, []int{160, 160}, []int{5, 5}, 0},
		{"20ms_control", core.CodecPCMA, 8000, 16000, 256, []int{160, 160}, []int{1, 1}, 128},
		{"old_maximum", core.CodecPCML, 16000, 8000, 8192, []int{65536}, []int{4}, 0},
		{"downsample_carry", core.CodecPCML, 16000, 8000, 2, []int{65533, 65536}, []int{16383, 16385}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			format := baichuan.TalkFormat{SampleRate: test.camera, SamplePrecision: 16, SamplesPerBlock: test.block}
			blocks := 0
			talk := &fakeTalk{format: format, writeFn: func(_ context.Context, block []byte) error {
				if len(block) != format.BytesPerBlock() {
					t.Fatalf("encoded block size: got %d, want %d", len(block), format.BytesPerBlock())
				}
				blocks++
				return nil
			}}
			b := &backchannel{producer: &Producer{ctx: ctx, cancel: cancel}, format: format, talk: talk}
			defer b.Close()
			if err := b.setCodec(&core.Codec{Name: test.codec, ClockRate: test.rate, Channels: 1}); err != nil {
				t.Fatal(err)
			}
			for i, n := range test.samples {
				if test.codec == core.CodecPCML {
					n *= 2
				}
				before := blocks
				if err := b.Write(make([]byte, n)); err != nil {
					t.Fatal(err)
				}
				if got := blocks - before; got != test.blocks[i] {
					t.Fatalf("packet %d: got %d blocks, want %d", i, got, test.blocks[i])
				}
				if len(b.samples) >= int(test.block) || cap(b.samples) > int(test.block) {
					t.Fatalf("block accumulator: len=%d cap=%d block=%d", len(b.samples), cap(b.samples), test.block)
				}
			}
			if len(b.samples) != test.leftover {
				t.Fatalf("leftover samples: got %d, want %d", len(b.samples), test.leftover)
			}
		})
	}
}
