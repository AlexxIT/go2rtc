package reolink

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestProducerResyncsAfterActiveMediaDiscontinuity(t *testing.T) {
	opener := &fakeProfileOpener{}
	producer, err := testRegistry(opener.open).Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Stop() })
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	waitFor(t, func() bool { return producer.profile.videoFrames.Load() == 1 })
	input := opener.input(0)
	for _, packet := range []baichuan.MediaPacket{
		{Kind: baichuan.MediaDiscontinuity}, testH264PFrame(1_050_000),
		testH264Keyframe(1_100_000), testH264PFrame(1_150_000),
	} {
		input.results <- inputResult{packet: packet}
	}
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if frames := producer.profile.videoFrames.Load(); frames != 3 {
		t.Fatalf("active discontinuity did not gate dependent P and recover I/P: frames=%d, want 3", frames)
	}
}

func TestProfileProbeDiscoversButSuppressesShortVideo(t *testing.T) {
	input := &fakeProfileInput{results: make(chan inputResult, 2), done: make(chan struct{})}
	short := testH264Keyframe(1_000_000)
	short.ShortVideo = true
	input.results <- inputResult{packet: short}
	input.results <- inputResult{packet: testAAC(1_000_000)}
	profile := &Profile{ctx: context.Background()}
	probe, err := profile.probe(input)
	if err != nil || profile.pipeline.videoMedia == nil || profile.pipeline.audioMedia == nil ||
		len(probe.packets) != 2 || !probe.packets[0].ShortVideo {
		t.Fatalf("shortened I did not retain codec discovery: packets=%+v err=%v", probe.packets, err)
	}
	for _, packet := range probe.packets {
		frame, frameErr := profile.pipeline.frame(packet)
		wantFrame := packet.Kind == baichuan.MediaAAC
		if frameErr != nil || frame.valid() != wantFrame {
			t.Fatalf("unexpected replay frame for kind %d: valid=%t err=%v", packet.Kind, frame.valid(), frameErr)
		}
	}
	if frame, err := profile.pipeline.frame(testH264PFrame(1_050_000)); err != nil || frame.valid() {
		t.Fatalf("dependent P escaped gate: frame=%v err=%v", frame, err)
	}
	if frame, err := profile.pipeline.frame(testH264Keyframe(3_000_000)); err != nil || !frame.valid() {
		t.Fatalf("next valid I did not restore video: frame=%v err=%v", frame, err)
	}
	if frame, err := profile.pipeline.frame(testH264PFrame(3_050_000)); err != nil || !frame.valid() {
		t.Fatalf("P after recovery was lost: frame=%v err=%v", frame, err)
	}
}

func TestProfileProbeRestartsAfterMediaDiscontinuity(t *testing.T) {
	input := &fakeProfileInput{results: make(chan inputResult, 4), done: make(chan struct{})}
	input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
	input.results <- inputResult{packet: baichuan.MediaPacket{Kind: baichuan.MediaDiscontinuity}}
	input.results <- inputResult{packet: testH264Keyframe(1_050_000)}
	input.results <- inputResult{packet: testAAC(1_050_000)}
	profile := &Profile{ctx: context.Background()}
	probe, err := profile.probe(input)
	if err != nil || len(probe.packets) != 2 || probe.packets[0].Timestamp != 1_050_000 ||
		profile.pipeline.videoMedia == nil {
		t.Fatalf("stale probe after discontinuity: packets=%+v err=%v", probe.packets, err)
	}
}

func TestProfileDiscontinuityGatesVideoUntilKeyframe(t *testing.T) {
	startCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Profile{ctx: context.Background(), startCtx: startCtx}
	key := testH264Keyframe(1_000_000)
	if err := p.pipeline.configureVideo(key); err != nil {
		t.Fatal(err)
	}
	p.initReceivers()
	defer p.closeReceivers()
	frames := 0
	(&core.Node{Input: func(*rtp.Packet) { frames++ }}).WithParent(&p.video.Node)
	probe := &mediaProbe{}
	if err := probe.retain(key); err != nil {
		t.Fatal(err)
	}
	input := &fakeProfileInput{results: make(chan inputResult, 2), done: make(chan struct{})}
	input.results <- inputResult{packet: baichuan.MediaPacket{Kind: baichuan.MediaDiscontinuity}}
	input.results <- inputResult{packet: testH264PFrame(1_050_000)}
	done := make(chan error, 1)
	go func() { done <- p.prepareStart(input, probe) }()
	waitFor(t, func() bool { return p.recvBytes.Load() != 0 })
	cancel()
	if err := <-done; err != nil || frames != 0 || len(probe.packets) != 1 ||
		probe.packets[0].Kind != baichuan.MediaVideoP {
		t.Fatalf("stale probe replayed after recovery: frames=%d packets=%d err=%v", frames, len(probe.packets), err)
	}
	if err := p.replay(*probe); err != nil || frames != 0 {
		t.Fatalf("dependent prestart P escaped recovery gate: frames=%d err=%v", frames, err)
	}
	if frame, err := p.pipeline.frame(testH264PFrame(1_050_000)); err != nil || frame.valid() {
		t.Fatalf("dependent frame escaped gate: frame=%v err=%v", frame, err)
	}
	if frame, err := p.pipeline.frame(testH264Keyframe(1_100_000)); err != nil || !frame.valid() {
		t.Fatalf("keyframe did not restore stream: frame=%v err=%v", frame, err)
	}
}
