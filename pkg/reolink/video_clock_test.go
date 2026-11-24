package reolink

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

func TestVideoResyncCounterCountsOnlySuccessfulRecovery(t *testing.T) {
	profile := &Producer{}
	pipeline := &mediaPipeline{videoResyncs: &profile.videoResyncs}
	base := testH264Keyframe(1_000_000)
	if err := pipeline.configureVideo(base); err != nil {
		t.Fatal(err)
	}
	if frame, err := pipeline.videoFrame(base); err != nil || !frame.valid() {
		t.Fatalf("initial keyframe failed: frame=%v err=%v", frame, err)
	}
	if got := profile.videoResyncs.Load(); got != 0 {
		t.Fatalf("startup keyframe counted as resync: %d", got)
	}
	short := testH264Keyframe(1_025_000)
	short.ShortVideo = true
	if frame, err := pipeline.videoFrame(short); err != nil || frame.valid() {
		t.Fatalf("short keyframe was not suppressed: frame=%v err=%v", frame, err)
	}
	if got := profile.videoResyncs.Load(); got != 0 {
		t.Fatalf("entering resync counted as recovery: %d", got)
	}
	if frame, err := pipeline.videoFrame(testH264Keyframe(1_050_000)); err != nil || !frame.valid() {
		t.Fatalf("valid recovery keyframe failed: frame=%v err=%v", frame, err)
	}
	if got := profile.videoResyncs.Load(); got != 1 {
		t.Fatalf("successful resyncs = %d, want 1", got)
	}
	if frame, err := pipeline.videoFrame(testH264PFrame(1_100_000)); err != nil || !frame.valid() {
		t.Fatalf("normal P-frame failed: frame=%v err=%v", frame, err)
	}
	if got := profile.videoResyncs.Load(); got != 1 {
		t.Fatalf("normal frame changed successful resync count: %d", got)
	}
}

func TestVideoClockConfirmation(t *testing.T) {
	for _, codec := range []string{"H264", "H265"} {
		t.Run(codec, func(t *testing.T) {
			for _, raw := range []uint32{1_000_000, 0xfffffff0} {
				m := mediaPipeline{videoOnly: true, epoch: mediaEpoch{video: 1<<32 - 100}}
				initial := testVideoKeyframe(codec, raw)
				if err := m.configureVideo(initial); err != nil {
					t.Fatal(err)
				}
				if _, err := m.videoFrame(initial); err != nil {
					t.Fatal(err)
				}
				m.videoTS.remainder = 700_000
				previous := m.videoTS
				m.suppressInvalidVideo()
				for _, packet := range []baichuan.MediaPacket{
					testVideoKeyframe(codec, raw+31_000_001),
					testVideoKeyframe(codec, raw+31_000_001),
					testVideoPFrame(codec, raw+31_050_001),
				} {
					frame, err := m.videoFrame(packet)
					if err != nil || frame.valid() || m.videoTS != previous || !m.videoPending.set {
						t.Fatalf("candidate changed accepted clock: frame=%v err=%v clock=%+v", frame, err, m.videoTS)
					}
				}
				delta := uint32(32_000_002)
				frame, err := m.videoFrame(testVideoKeyframe(codec, raw+delta))
				scaled := uint64(delta)*90000 + previous.remainder
				want := previous.value + scaled/1_000_000
				if err != nil || !frame.valid() || frame.packet.Timestamp != uint32(want) ||
					m.videoTS.value != want || m.videoTS.remainder != scaled%1_000_000 ||
					m.videoTS.last != raw+delta || m.videoResync || m.videoPending.set {
					t.Fatalf("confirmation lost full clock interval: frame=%v err=%v clock=%+v", frame, err, m.videoTS)
				}
				if frame, err = m.videoFrame(testVideoPFrame(codec, raw+delta+50_000)); err != nil || !frame.valid() {
					t.Fatalf("confirmed video did not resume: frame=%v err=%v", frame, err)
				}
			}
		})
	}
}

func TestVideoClockCandidateContradictions(t *testing.T) {
	for _, name := range []string{"old P", "old I", "short", "invalid", "parameters", "P parameters", "discontinuity", "large P"} {
		t.Run(name, func(t *testing.T) {
			m := mediaPipeline{videoOnly: true}
			if err := m.configureVideo(testH264Keyframe(1_000_000)); err != nil {
				t.Fatal(err)
			}
			if _, err := m.videoFrame(testH264Keyframe(1_000_000)); err != nil {
				t.Fatal(err)
			}
			previous := m.videoTS
			m.suppressInvalidVideo()
			if frame, err := m.videoFrame(testH264Keyframe(32_000_000)); err != nil || frame.valid() {
				t.Fatalf("candidate was not suppressed: frame=%v err=%v", frame, err)
			}
			packet := testH264PFrame(32_050_000)
			switch name {
			case "old P":
				packet.Timestamp = 1_050_000
			case "old I":
				packet = testH264Keyframe(1_100_000)
			case "short":
				packet.ShortVideo = true
			case "invalid":
				packet.Data = []byte{0, 0, 0, 1, 0xc1, 1}
			case "parameters":
				packet = testH264Keyframe(33_000_000)
				packet.Data[5] ^= 1
			case "P parameters":
				packet = testH264Keyframe(32_050_000)
				packet.Kind = baichuan.MediaVideoP
			case "discontinuity":
				m.suppressInvalidVideo()
			case "large P":
				packet.Timestamp = 63_000_001
			}
			frame, err := m.videoFrame(packet)
			if err != nil || m.videoPending.set {
				t.Fatalf("contradiction retained candidate: pending=%+v err=%v", m.videoPending, err)
			}
			if name == "old I" {
				if !frame.valid() || m.videoTS.last != packet.Timestamp || m.videoResync {
					t.Fatal("old timeline did not resume")
				}
			} else if frame.valid() || m.videoTS != previous {
				t.Fatal("contradiction changed accepted clock")
			}
		})
	}
}

func TestVideoClockConfirmationLimits(t *testing.T) {
	for _, test := range []struct {
		name       string
		videoOnly  bool
		resync     bool
		first      uint32
		second     uint32
		firstError bool
		wantError  bool
	}{
		{"audio enabled", false, true, 31_000_000, 32_000_000, true, true},
		{"healthy video", true, false, 31_000_000, 32_000_000, true, true},
		{"over ceiling", true, true, 60_000_001, 61_000_000, true, true},
		{"ceiling", true, true, 59_000_000, 60_000_000, false, false},
		{"confirmation over ceiling", true, true, 59_000_000, 60_000_001, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := mediaPipeline{videoOnly: test.videoOnly}
			if err := m.configureVideo(testH264Keyframe(1_000_000)); err != nil {
				t.Fatal(err)
			}
			if _, err := m.videoFrame(testH264Keyframe(1_000_000)); err != nil {
				t.Fatal(err)
			}
			m.videoResync = test.resync
			previous := m.videoTS
			frame, err := m.videoFrame(testH264Keyframe(1_000_000 + test.first))
			if (err != nil) != test.firstError || frame.valid() || m.videoTS != previous {
				t.Fatalf("unexpected first decision: frame=%v err=%v", frame, err)
			}
			frame, err = m.videoFrame(testH264Keyframe(1_000_000 + test.second))
			if (err != nil) != test.wantError || frame.valid() == test.wantError {
				t.Fatalf("unexpected confirmation decision: frame=%v err=%v", frame, err)
			}
		})
	}
}

func TestProfileClockConfirmationRequiresConfiguredVideoOnly(t *testing.T) {
	for _, query := range []string{"", "?audio=0", "?audio=0&video=0"} {
		s, err := parseURL("reolink://user:pass@camera/main" + query)
		if err != nil {
			t.Fatal(err)
		}
		camera := &Camera{epochs: make(map[ProfileKey]mediaEpoch)}
		p := newProducer(camera, ProfileKey{}, 1, s, nil)
		want := query == "?audio=0"
		if p.pipeline.videoOnly != want || p.audio != nil {
			t.Fatalf("configured source eligibility=%t, want %t", p.pipeline.videoOnly, want)
		}
		if want {
			input := &fakeProfileInput{results: make(chan inputResult, 2), done: make(chan struct{})}
			input.results <- inputResult{packet: baichuan.MediaPacket{Kind: baichuan.MediaDiscontinuity}}
			input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
			if _, err = p.probe(input); err != nil || !p.pipeline.videoOnly {
				t.Fatalf("probe reset lost configured eligibility: err=%v", err)
			}
		}
		p.cancel()
	}
}
