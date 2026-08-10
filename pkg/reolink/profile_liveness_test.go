package reolink

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

const (
	testTrackCheckInterval = 10 * time.Millisecond
	testTrackStallTimeout  = 100 * time.Millisecond
)

func TestProfileFailsWhenAdvertisedTrackStalls(t *testing.T) {
	for _, test := range []struct {
		name     string
		packet   func(uint32) baichuan.MediaPacket
		want     string
		closeErr error
	}{
		{name: "video", packet: testAAC, want: "video track stalled"},
		{name: "audio", packet: testH264PFrame, want: "audio track stalled", closeErr: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			opener := &fakeProfileOpener{}
			registry := testRegistry(opener.open)
			producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
			if err != nil {
				t.Fatal(err)
			}
			producer.profile.trackCheckInterval = testTrackCheckInterval
			producer.profile.trackStallTimeout = testTrackStallTimeout
			done := make(chan error, 1)
			go func() { done <- producer.Start() }()
			input := opener.input(0)
			input.closeErr = test.closeErr
			ticker := time.NewTicker(testTrackCheckInterval)
			defer ticker.Stop()
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			for timestamp := uint32(1_050_000); ; timestamp += 5_000 {
				select {
				case err = <-done:
					if err == nil || !strings.Contains(err.Error(), test.want) {
						t.Fatalf("unexpected terminal error: %v", err)
					}
					if class := errorClass(err); test.closeErr == nil && class != "media" {
						t.Fatalf("track stall classified as %q", class)
					}
					if class := producer.diagnostics().LastError; class != "media" {
						t.Fatalf("profile recorded track stall as %q", class)
					}
					if err = producer.Stop(); !errors.Is(err, test.closeErr) {
						t.Fatalf("unexpected stop error: %v", err)
					}
					return
				case <-ticker.C:
					input.results <- inputResult{packet: test.packet(timestamp)}
				case <-deadline.C:
					t.Fatal("track stall did not stop producer")
				}
			}
		})
	}
}

func TestProfileDoesNotMonitorUnadvertisedAudio(t *testing.T) {
	profile := deliveryTestProfile()
	profile.trackCheckInterval = testTrackCheckInterval
	profile.trackStallTimeout = testTrackStallTimeout
	ctx, cancel := context.WithCancel(context.Background())
	stalled := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		profile.watchTracks(ctx, cancel, stalled)
		close(done)
	}()
	for range 15 {
		profile.videoFrames.Add(1)
		time.Sleep(testTrackCheckInterval)
	}
	select {
	case err := <-stalled:
		t.Fatalf("unadvertised audio stopped video-only profile: %v", err)
	default:
	}
	cancel()
	<-done
}

func TestBackchannelOnlyProfileStallsWithoutMedia(t *testing.T) {
	format := baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 1016}
	opener := &fakeProfileOpener{talk: &format}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/sub?video=0&audio=0")
	if err != nil {
		t.Fatal(err)
	}
	producer.profile.trackCheckInterval = testTrackCheckInterval
	producer.profile.trackStallTimeout = testTrackStallTimeout
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	select {
	case err = <-done:
		if err == nil || !strings.Contains(err.Error(), "media stream stalled") {
			t.Fatalf("unexpected terminal error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("backchannel-only media stall did not stop producer")
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileReconnectsAfterLowAudioRate(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	const source = "reolink://admin:secret@camera/main?backchannel=0"
	first, err := registry.Dial(source)
	if err != nil {
		t.Fatal(err)
	}
	setTestTrackHealth(first.profile, 300*time.Millisecond, 500*time.Millisecond)
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.Start() }()
	input := opener.input(0)
	err = runProfileTraffic(input, firstDone, 20*time.Millisecond, 200*time.Millisecond, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "audio track rate below expected") {
		t.Fatalf("unexpected under-rate result: %v", err)
	}
	if class := first.diagnostics().LastError; class != "media" {
		t.Fatalf("under-rate failure classified as %q", class)
	}

	replacement, err := registry.Dial(source)
	if err != nil {
		t.Fatalf("replacement profile failed: %v", err)
	}
	setTestTrackHealth(replacement.profile, 300*time.Millisecond, 300*time.Millisecond)
	replacementDone := make(chan error, 1)
	go func() { replacementDone <- replacement.Start() }()
	input = opener.input(1)
	if err = runProfileTraffic(input, replacementDone, 20*time.Millisecond, 50*time.Millisecond, time.Second); err != nil {
		t.Fatalf("full-rate replacement stopped: %v", err)
	}
	if err = first.Stop(); err != nil {
		t.Fatalf("failed to stop terminal profile: %v", err)
	}
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-replacementDone; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected replacement terminal error: %v", err)
	}
	if err = replacement.Stop(); err != nil {
		t.Fatal(err)
	}
}

func setTestTrackHealth(profile *Profile, stall, rateWindow time.Duration) {
	profile.trackCheckInterval = 5 * time.Millisecond
	profile.trackStallTimeout = stall
	profile.trackRateWindow = rateWindow
}

func runProfileTraffic(
	input *fakeProfileInput, done <-chan error, videoEvery, audioEvery, duration time.Duration,
) error {
	video := time.NewTicker(videoEvery)
	defer video.Stop()
	audio := time.NewTicker(audioEvery)
	defer audio.Stop()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	var timestamp uint32 = 1_050_000
	for {
		select {
		case err := <-done:
			return err
		case <-video.C:
			input.results <- inputResult{packet: testH264PFrame(timestamp)}
			timestamp += 5_000
		case <-audio.C:
			input.results <- inputResult{packet: testAAC(timestamp)}
		case <-deadline.C:
			return nil
		}
	}
}

func TestProfileIgnoresStalledSuppressedTrack(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?video=0&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	producer.profile.trackCheckInterval = testTrackCheckInterval
	producer.profile.trackStallTimeout = testTrackStallTimeout
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	input := opener.input(0)
	ticker := time.NewTicker(testTrackCheckInterval)
	defer ticker.Stop()
	stable := time.NewTimer(250 * time.Millisecond)
	defer stable.Stop()
	for timestamp := uint32(1_050_000); ; timestamp += 5_000 {
		select {
		case err = <-done:
			t.Fatalf("suppressed video stall stopped producer: %v", err)
		case <-ticker.C:
			input.results <- inputResult{packet: testH264PFrame(timestamp)}
			input.results <- inputResult{packet: testAAC(timestamp)}
		case <-stable.C:
			goto passed
		}
	}
passed:
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStallsOnEnabledTrackOnly(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?video=0&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	producer.profile.trackCheckInterval = testTrackCheckInterval
	producer.profile.trackStallTimeout = testTrackStallTimeout
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case err = <-done:
			if err == nil || !strings.Contains(err.Error(), "audio track stalled") {
				t.Fatalf("unexpected terminal error: %v", err)
			}
			if err = producer.Stop(); err != nil {
				t.Fatalf("unexpected stop error: %v", err)
			}
			return
		case <-deadline.C:
			t.Fatal("audio stall did not stop producer")
		}
	}
}
