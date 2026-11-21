package reolink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

func TestProfileProbeSkipsInvalidInitialAudio(t *testing.T) {
	aac := testAAC(1_000_000)
	adpcm := baichuan.MediaPacket{Kind: baichuan.MediaADPCM, Data: []byte{0, 0, 0, 0, 0}}
	for _, test := range []struct {
		name       string
		bad, valid baichuan.MediaPacket
		samples    uint64
	}{
		{"AAC header", baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: aac.Data[:2]}, aac, 1024},
		{"AAC size", baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: aac.Data[:9]}, aac, 1024},
		{"ADPCM", baichuan.MediaPacket{Kind: baichuan.MediaADPCM, Data: []byte{0, 0, 89, 0, 0}}, adpcm, 2},
	} {
		for _, afterI := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/afterI=%t", test.name, afterI), func(t *testing.T) {
				input := &fakeProfileInput{results: make(chan inputResult, 3), done: make(chan struct{})}
				video := testH264Keyframe(1_000_000)
				if afterI {
					input.results <- inputResult{packet: video}
				}
				input.results <- inputResult{packet: test.bad}
				if !afterI {
					input.results <- inputResult{packet: video}
				}
				input.results <- inputResult{packet: test.valid}
				p := &Producer{source: &source{}, ctx: context.Background()}
				probe, err := p.probe(input)
				if err != nil {
					t.Fatal(err)
				}
				defer probe.release()
				if len(probe.packets) != 2 || probe.packets[0].Kind != baichuan.MediaVideoI ||
					probe.packets[1].Kind != test.valid.Kind || !bytes.Equal(probe.packets[1].Data, test.valid.Data) {
					t.Fatalf("probe retained invalid audio: packets=%d", len(probe.packets))
				}
				p.initReceivers()
				defer p.closeReceivers()
				if err = p.replay(probe); err != nil {
					t.Fatal(err)
				}
				if p.videoFrames.Load() != 1 || p.audioSamples.Load() != test.samples {
					t.Fatalf("replayed video=%d audio=%d", p.videoFrames.Load(), p.audioSamples.Load())
				}
			})
		}
	}
}

func TestProfileProbeInvalidAudioWaitsForDeadline(t *testing.T) {
	for _, kind := range []baichuan.MediaKind{baichuan.MediaAAC, baichuan.MediaADPCM} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			input := &fakeProfileInput{results: make(chan inputResult, 5), done: make(chan struct{})}
			input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
			for range 4 {
				input.results <- inputResult{packet: baichuan.MediaPacket{Kind: kind, Data: []byte{1}}}
			}
			p := &Producer{source: &source{}, ctx: ctx}
			_, err := p.probe(input)
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "waiting for audio") {
				t.Fatalf("invalid initial audio did not await bounded discovery: %v", err)
			}
			if p.pipeline.audioMedia != nil {
				t.Fatal("invalid audio configured a track")
			}
		})
	}
}

func TestProfileProbeMissingTrackErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		packet baichuan.MediaPacket
		cause  error
	}{
		{"video and audio", baichuan.MediaPacket{}, io.EOF},
		{"audio", testH264Keyframe(1_000_000), context.DeadlineExceeded},
		{"video", testAAC(1_000_000), context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &fakeProfileInput{results: make(chan inputResult, 2), done: make(chan struct{})}
			if test.packet.Kind != 0 {
				input.results <- inputResult{packet: test.packet}
			}
			input.results <- inputResult{err: test.cause}
			p := &Producer{source: &source{}, ctx: context.Background()}
			_, err := p.probe(input)
			if !errors.Is(err, test.cause) || !strings.Contains(err.Error(), "waiting for "+test.name+":") {
				t.Fatalf("missing-track error lost detail or cause: %v", err)
			}
		})
	}
}

func TestProfileProbeAudioOptOut(t *testing.T) {
	s, err := parseURL("reolink://u:p@cam/main?audio=0")
	if err != nil {
		t.Fatal(err)
	}
	input := &fakeProfileInput{results: make(chan inputResult, 1), done: make(chan struct{})}
	input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
	p := &Producer{ctx: context.Background(), source: &s}
	probe, err := p.probe(input)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.release()
	if p.pipeline.videoMedia == nil || p.pipeline.audioMedia != nil || len(probe.packets) != 1 {
		t.Fatal("audio opt-out required or retained an audio track")
	}
}

func TestProbeVideoKeepsH264SPSWithNALRefIDC(t *testing.T) {
	data := h264AccessUnit(
		[]byte{0x47, 0x64, 0x00, 0x1f}, []byte{0x68, 0xee}, []byte{0x65, 0x80},
	)
	original := bytes.Clone(data)
	pipeline := &mediaPipeline{}
	if err := pipeline.probeVideo(baichuan.MediaPacket{Codec: "H264", Data: data}); err != nil {
		t.Fatal(err)
	}
	if pipeline.videoMedia == nil {
		t.Fatal("probe discarded the legal H264 SPS with nal_ref_idc=2")
	}
	if !bytes.Equal(data, original) {
		t.Fatal("probe modified borrowed packet data")
	}
}
