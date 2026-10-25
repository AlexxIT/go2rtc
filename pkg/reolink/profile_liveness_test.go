package reolink

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
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
	err = runProfileTraffic(input, firstDone, 20*time.Millisecond, 200*time.Millisecond, 20*time.Millisecond, 2*time.Second)
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
	if err = runProfileTraffic(input, replacementDone, 20*time.Millisecond, 50*time.Millisecond, 20*time.Millisecond, time.Second); err != nil {
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
	input *fakeProfileInput, done <-chan error, videoEvery, audioEvery, videoStep, duration time.Duration,
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
			timestamp += uint32(videoStep / time.Microsecond)
		case <-audio.C:
			input.results <- inputResult{packet: testAAC(timestamp)}
		case <-deadline.C:
			return nil
		}
	}
}

func TestProfileAudioRateFollowsVideoProgress(t *testing.T) {
	for _, test := range []struct {
		name       string
		audioEvery time.Duration
		low        bool
	}{
		{name: "common backlog", audioEvery: 128 * time.Millisecond},
		{name: "disproportionately slow audio", audioEvery: 384 * time.Millisecond, low: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			opener := &fakeProfileOpener{}
			producer, err := testRegistry(opener.open).Dial("reolink://admin:secret@camera/main?backchannel=0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = producer.Stop() })
			setTestTrackHealth(producer.profile, 500*time.Millisecond, time.Second)
			done := make(chan error, 1)
			go func() { done <- producer.Start() }()
			input := opener.input(0)
			// One AAC packet and one video step each represent 64ms of content.
			err = runProfileTraffic(input, done, 128*time.Millisecond, test.audioEvery, 64*time.Millisecond, 2200*time.Millisecond)
			if test.low {
				if err == nil || !strings.Contains(err.Error(), "audio track rate below expected") {
					t.Fatalf("audio starvation was not detected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("common backlog terminated: %v", err)
			}
			input.results <- inputResult{err: io.EOF}
			if err = <-done; !errors.Is(err, io.EOF) {
				t.Fatalf("unexpected terminal error: %v", err)
			}
		})
	}
}

func TestAudioRateWindowWrapAndRecovery(t *testing.T) {
	for _, initial := range []uint32{0, 0xffff0000} {
		profile := &Profile{video: &core.Receiver{}, pipeline: mediaPipeline{audioRate: 16000}, trackRateWindow: time.Minute}
		start := time.Now()
		window := audioRateWindow{at: start, video: uint64(initial), videoSet: true}
		profile.videoRate.Store(uint64(initial + 30*90000))
		profile.audioSamples.Store(30 * 16000)
		if profile.checkAudioRate(start.Add(time.Minute), 600, &window) {
			t.Fatalf("common backlog failed at initial RTP timestamp %d", initial)
		}

		// An accepted recovery anchors both tracks, including a valid zero RTP timestamp.
		profile.videoRate.Store(2 << 32)
		profile.audioSamples.Store(30*16000 + 1)
		now := start.Add(2 * time.Minute)
		if profile.checkAudioRate(now, 601, &window) || window.at != now ||
			window.video != 2<<32 || window.samples != 30*16000+1 || !window.videoSet {
			t.Fatalf("recovery did not reset both rate baselines: %+v", window)
		}
		profile.videoRate.Store(2<<32 | 60*90000)
		if !profile.checkAudioRate(now.Add(time.Minute), 1800, &window) {
			t.Fatal("stable healthy video hid audio starvation after recovery")
		}
	}
}

func TestAudioRateWindowAnchorsRecoveredVideo(t *testing.T) {
	profile := &Profile{trackRateWindow: time.Minute}
	base := testH264Keyframe(1_000_000)
	if err := profile.pipeline.configureVideo(base); err != nil {
		t.Fatal(err)
	}
	if err := profile.pipeline.configureAudio(testAAC(1_000_000)); err != nil {
		t.Fatal(err)
	}
	profile.initReceivers()
	t.Cleanup(profile.closeReceivers)
	if err := profile.writePacket(base); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	window := audioRateWindow{at: start, video: profile.videoRate.Load(), videoSet: true}
	profile.audioSamples.Store(20 * 16000)
	short := testH264PFrame(1_050_000)
	short.ShortVideo = true
	if err := profile.writePacket(short); err != nil {
		t.Fatal(err)
	}
	if profile.checkAudioRate(start, profile.videoFrames.Load(), &window) || window.videoSet {
		t.Fatal("suppressed video supplied a trusted rate anchor")
	}
	if err := profile.writePacket(testH264Keyframe(21_000_000)); err != nil {
		t.Fatal(err)
	}
	recovered := start.Add(20 * time.Second)
	if profile.checkAudioRate(recovered, profile.videoFrames.Load(), &window) || !window.videoSet ||
		uint32(window.video) != 20*90000 || window.samples != 20*16000 || window.at != recovered {
		t.Fatalf("accepted recovery did not reanchor both tracks: %+v", window)
	}
	if err := profile.writePacket(testH264PFrame(51_000_000)); err != nil {
		t.Fatal(err)
	}
	profile.audioSamples.Add(30 * 16000)
	if profile.checkAudioRate(recovered.Add(time.Minute), profile.videoFrames.Load(), &window) {
		t.Fatal("recovery jump was counted against subsequent intact common backlog")
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
