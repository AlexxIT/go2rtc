package reolink

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
)

func talkPCM(samples []int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(sample))
	}
	return b
}

func TestTalkInterpolation(t *testing.T) {
	samples := make([]int16, 512)
	for i := range samples {
		samples[i] = int16(12000 * math.Sin(2*math.Pi*3400*float64(i)/8000))
	}
	src := talkPCM(samples)
	var whole, split talkInterpolator
	want := whole.convert(src)
	got := append(split.convert(src[:14]), split.convert(src[14:318])...)
	got = append(got, split.convert(src[318:])...)
	if !bytes.Equal(got, want) {
		t.Fatal("packet boundaries changed interpolation")
	}
	// Project the settled output onto the desired tone and its 4.6 kHz image.
	var amplitude [2]float64
	for j, frequency := range []float64{3400, 4600} {
		var re, im float64
		for i := 64; i < len(want)/2; i++ {
			sample := float64(int16(binary.LittleEndian.Uint16(want[2*i:])))
			angle := 2 * math.Pi * frequency * float64(i) / 16000
			re += sample * math.Cos(angle)
			im += sample * math.Sin(angle)
		}
		amplitude[j] = math.Hypot(re, im)
	}
	if ratio := amplitude[1] / amplitude[0]; ratio > math.Pow(10, -30.0/20) {
		t.Fatalf("3.4 kHz image ratio = %g, want <= -30 dB", ratio)
	}
	for _, level := range []int16{-32768, -1000, 1000, 32767} {
		for i := range samples {
			samples[i] = level
		}
		var f talkInterpolator
		out := f.convert(talkPCM(samples))
		for i := 64; i < len(out)/2; i++ {
			if got := int16(binary.LittleEndian.Uint16(out[2*i:])); got != level {
				t.Fatalf("DC %d at %d = %d", level, i, got)
			}
		}
	}
	var impulse talkInterpolator
	out := impulse.convert(talkPCM([]int16{10000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
	for i, coefficient := range talkHalfband {
		if got := int16(binary.LittleEndian.Uint16(out[4*i:])); got != int16(math.Round(10000*coefficient)) {
			t.Fatalf("impulse phase at %d = %d", i, got)
		}
	}
	if got := int16(binary.LittleEndian.Uint16(out[30:])); got != 10000 {
		t.Fatalf("impulse center = %d", got)
	}
	impulse = talkInterpolator{}
	if got := impulse.convert(make([]byte, 64)); !bytes.Equal(got, make([]byte, 128)) {
		t.Fatal("reset retained history")
	}
	var step talkInterpolator
	stepOut := step.convert(talkPCM([]int16{32767, 32767, 32767, 32767, 32767, 32767, 32767, 32767, 32767}))
	// The Kaiser step overshoots at this phase. Saturation must precede int16 conversion.
	if got := int16(binary.LittleEndian.Uint16(stepOut[32:])); got != 32767 {
		t.Fatalf("positive step saturation = %d", got)
	}
}

func TestBackchannelAudioPaths(t *testing.T) {
	for _, codec := range talkCodecs() {
		for _, target := range []uint32{8000, 16000} {
			t.Run(codec.String()+"/"+(&core.Codec{ClockRate: target}).String(), func(t *testing.T) {
				format := baichuan.TalkFormat{SampleRate: target, SamplesPerBlock: 4}
				var got [][]byte
				b := &backchannel{producer: &Producer{stopCtx: context.Background()}, format: format}
				defer b.Close()
				b.start = func(context.Context) (talkSession, error) {
					return &fakeTalk{format: format, writeFn: func(_ context.Context, p []byte) error {
						got = append(got, append([]byte(nil), p...))
						return nil
					}}, nil
				}
				if err := b.setCodec(codec); err != nil {
					t.Fatal(err)
				}
				payload := bytes.Repeat([]byte{0x41}, 32)
				decodeTarget := target
				if codec.ClockRate == 8000 && target == 16000 {
					decodeTarget = 8000
				}
				decoded := pcm.Transcode(&core.Codec{Name: core.CodecPCML, ClockRate: decodeTarget, Channels: 1}, codec)(payload)
				if decodeTarget != target {
					var f talkInterpolator
					decoded = f.convert(decoded)
				}
				var encoder baichuan.ADPCMEncoder
				var want [][]byte
				for len(decoded) >= 8 {
					var samples [4]int16
					for i := range samples {
						samples[i] = int16(binary.LittleEndian.Uint16(decoded[2*i:]))
					}
					block, err := encoder.EncodeBlock(samples[:])
					if err != nil {
						t.Fatal(err)
					}
					want = append(want, block)
					decoded = decoded[8:]
				}
				if err := b.Write(payload); err != nil {
					t.Fatal(err)
				}
				if len(got) != len(want) {
					t.Fatalf("blocks %d, want %d", len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("block %d differs", i)
					}
				}
			})
		}
	}
}
