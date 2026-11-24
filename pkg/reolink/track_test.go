package reolink

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestAudioOnlyProfileRetainsLatestStartupPacket(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?video=0&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	if len(producer.Medias) != 1 || producer.Medias[0].Kind != core.KindAudio ||
		len(producer.Receivers) != 1 || producer.video != nil || producer.audio == nil {
		t.Fatal("video-suppressed profile advertised unexpected tracks")
	}
	input := opener.input(0)
	bytes := producer.recvBytes.Load()
	const packets = maxProbePackets + 16
	for i := range packets {
		packet := testAAC(uint32(i) * 64_000)
		packet.Data[len(packet.Data)-1] = byte(i)
		input.results <- inputResult{packet: packet}
	}
	waitFor(t, func() bool {
		return producer.recvBytes.Load() == bytes+packets*uint64(len(testAAC(0).Data))
	})
	select {
	case <-producer.done:
		t.Fatal("audio-only profile failed before start")
	default:
	}

	audio := make(chan *rtp.Packet, 2)
	(&core.Node{Input: func(packet *rtp.Packet) { audio <- packet }}).WithParent(&producer.Receivers[0].Node)
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	select {
	case packet := <-audio:
		want := byte((packets - 1) % 256)
		if got := packet.Payload[len(packet.Payload)-1]; got != want {
			t.Fatalf("replayed audio payload = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("latest startup audio packet was not replayed")
	}
	select {
	case <-audio:
		t.Fatal("replayed stale startup audio")
	case <-time.After(10 * time.Millisecond):
	}
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileDeliversOnlyEnabledTracks(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?audio=0&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	if len(producer.Medias) != 1 || producer.Medias[0].Kind != core.KindVideo ||
		len(producer.Receivers) != 1 || producer.video == nil || producer.audio != nil {
		t.Fatal("audio-suppressed profile advertised unexpected tracks")
	}
	videoPackets := make(chan *rtp.Packet, 11)
	(&core.Node{Input: func(packet *rtp.Packet) { videoPackets <- packet }}).WithParent(&producer.Receivers[0].Node)
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	input := opener.input(0)
	for i := uint32(0); i < 10; i++ {
		input.results <- inputResult{packet: testH264PFrame(1_000_000 + i*50_000)}
		input.results <- inputResult{packet: testAAC(1_000_000 + i*50_000)}
	}
	for range 11 {
		select {
		case <-videoPackets:
		case <-time.After(time.Second):
			t.Fatal("video packet not delivered")
		}
	}
	if video, audio := producer.videoFrames.Load(), producer.audioSamples.Load(); video != 11 || audio != 0 {
		t.Fatalf("unexpected liveness state: video=%d audio_samples=%d", video, audio)
	}
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryBackchannelOnlySource(t *testing.T) {
	format := baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 1016}
	opener := &fakeProfileOpener{talk: &format}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/sub?video=0&audio=false")
	if err != nil {
		t.Fatal(err)
	}
	if len(producer.Receivers) != 0 || len(producer.Medias) != 1 ||
		producer.Medias[0].Direction != core.DirectionSendonly {
		t.Fatalf("unexpected backchannel-only medias=%v receivers=%d", producer.Medias, len(producer.Receivers))
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
	if !opener.input(0).closed.Load() {
		t.Fatal("backchannel-only stop returned before preview closed")
	}
}

func TestRegistryRejectsUnsupportedBackchannelOnlySource(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	_, err := registry.Dial("reolink://admin:secret@camera/sub?video=0&audio=0")
	if err == nil || !strings.Contains(err.Error(), "requires camera talkback") {
		t.Fatalf("unexpected unsupported backchannel result: %v", err)
	}
	if opener.count() != 1 || !opener.input(0).closed.Load() {
		t.Fatal("unsupported backchannel-only source leaked its preview")
	}
	registry.mu.Lock()
	cameras := len(registry.cameras)
	registry.mu.Unlock()
	if cameras != 0 {
		t.Fatalf("unsupported backchannel-only source retained %d cameras", cameras)
	}
}
