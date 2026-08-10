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

func TestParseURLAcceptsTrackSuppression(t *testing.T) {
	for _, tc := range []struct {
		name      string
		url       string
		wantVideo bool
		wantAudio bool
	}{
		{name: "default_both", url: "reolink://u:p@cam/main", wantVideo: true, wantAudio: true},
		{name: "video_zero", url: "reolink://u:p@cam/main?video=0", wantAudio: true},
		{name: "audio_zero", url: "reolink://u:p@cam/main?audio=0", wantVideo: true},
		{name: "video_false", url: "reolink://u:p@cam/main?video=false", wantAudio: true},
		{name: "audio_false", url: "reolink://u:p@cam/main?audio=false", wantVideo: true},
		{name: "video_one", url: "reolink://u:p@cam/main?video=1", wantVideo: true, wantAudio: true},
		{name: "audio_one", url: "reolink://u:p@cam/main?audio=1", wantVideo: true, wantAudio: true},
		{name: "video_true", url: "reolink://u:p@cam/main?video=true", wantVideo: true, wantAudio: true},
		{name: "audio_true", url: "reolink://u:p@cam/main?audio=true", wantVideo: true, wantAudio: true},
		{name: "combined", url: "reolink://u:p@cam/main?video=0&audio=1", wantAudio: true},
		{name: "backchannel_only", url: "reolink://u:p@cam/main?video=0&audio=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseURL(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if !s.suppressVideo != tc.wantVideo || !s.suppressAudio != tc.wantAudio {
				t.Fatalf("unexpected tracks: video=%t audio=%t", !s.suppressVideo, !s.suppressAudio)
			}
		})
	}
}

func TestParseURLRejectsInvalidTrackSelection(t *testing.T) {
	for _, source := range []string{
		"reolink://u:p@cam/main?video=0&audio=0&backchannel=0",
		"reolink://u:p@cam/main?video=2",
		"reolink://u:p@cam/main?audio=yes",
		"reolink://u:p@cam/main?video=0&video=1",
	} {
		if _, err := parseURL(source); err == nil {
			t.Fatalf("accepted invalid source: %s", source)
		}
	}
}

func TestAudioOnlyProfileRetainsLatestStartupPacket(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?video=0&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	if len(producer.Medias) != 1 || producer.Medias[0].Kind != core.KindAudio ||
		len(producer.Receivers) != 1 || producer.profile.video != nil || producer.profile.audio == nil {
		t.Fatal("video-suppressed profile advertised unexpected tracks")
	}
	input := opener.input(0)
	bytes := producer.profile.recvBytes.Load()
	const packets = maxProbePackets + 16
	for i := range packets {
		packet := testAAC(uint32(i) * 64_000)
		packet.Data[len(packet.Data)-1] = byte(i)
		input.results <- inputResult{packet: packet}
	}
	waitFor(t, func() bool {
		return producer.profile.recvBytes.Load() == bytes+packets*uint64(len(testAAC(0).Data))
	})
	select {
	case <-producer.profile.done:
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
		len(producer.Receivers) != 1 || producer.profile.video == nil || producer.profile.audio != nil {
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
	if video, audio := producer.profile.videoFrames.Load(), producer.profile.audioSamples.Load(); video != 11 || audio != 0 {
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
