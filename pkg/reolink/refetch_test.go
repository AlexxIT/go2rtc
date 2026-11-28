package reolink

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestProducerRefetch(t *testing.T) {
	for _, scenario := range []string{
		"resume", "different audio", "video skip", "cancel",
		"cadence resume", "cadence trial skip", "cadence drain skip", "cadence long phase skip",
	} {
		t.Run(scenario, func(t *testing.T) {
			variable := strings.HasPrefix(scenario, "cadence ")
			resumes := scenario == "resume" || scenario == "cadence resume" || scenario == "cadence drain skip"
			live := newFakeProfileInput()
			history := &fakeProfileInput{results: make(chan inputResult, 32), done: make(chan struct{})}
			request := make(chan source, 1)
			canceled := make(chan struct{})
			registry := testRegistry(func(ctx context.Context, _ *Camera, s source) (profileInput, error) {
				if s.preRecord == 0 {
					return live, nil
				}
				if !live.aborted.Load() {
					return nil, errors.New("history overlapped original connection")
				}
				request <- s
				if scenario == "cancel" {
					<-ctx.Done()
					close(canceled)
					return nil, ctx.Err()
				}
				return history, nil
			})
			producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0&options=refetch")
			if err != nil {
				t.Fatal(err)
			}
			defer producer.Stop()
			video := make(chan *rtp.Packet, 32)
			audio := make(chan *rtp.Packet, 32)
			(&core.Node{Input: func(packet *rtp.Packet) { video <- packet }}).WithParent(&producer.video.Node)
			(&core.Node{Input: func(packet *rtp.Packet) { audio <- packet }}).WithParent(&producer.audio.Node)
			get := func(packets <-chan *rtp.Packet) *rtp.Packet {
				t.Helper()
				select {
				case packet := <-packets:
					return packet
				case <-time.After(time.Second):
					t.Fatal("no packet on original receiver")
					return nil
				}
			}
			done := make(chan error, 1)
			go func() { done <- producer.Start() }()
			lastVideo, lastAudio := get(video), get(audio)
			raw := uint32(1_000_000)
			for i := range 8 {
				if variable {
					raw += [...]uint32{100_000, 50_000, 50_000}[i%3]
				} else {
					raw += 64_000
				}
				live.results <- inputResult{packet: testH264PFrame(raw)}
				aac := testAAC(0)
				aac.Data[len(aac.Data)-1] = byte(i + 1)
				live.results <- inputResult{packet: aac}
				lastVideo, lastAudio = get(video), get(audio)
				history.results <- inputResult{packet: testH264PFrame(raw)}
				aac = testAAC(0)
				aac.Data[len(aac.Data)-1] = byte(i + 1)
				if scenario == "different audio" && i == 7 {
					aac.Data[len(aac.Data)-1]++
				}
				history.results <- inputResult{packet: aac}
			}
			nextSteps := []uint32{64_000}
			if variable {
				nextSteps = []uint32{50_000, 100_000}
			}
			if scenario == "video skip" {
				nextSteps[0] += 1_000_000
			}
			if scenario == "cadence long phase skip" {
				nextSteps[1] += 50_000
			}
			for _, step := range nextSteps {
				raw += step
				history.results <- inputResult{packet: testH264PFrame(raw)}
			}
			if scenario == "cadence trial skip" {
				history.results <- inputResult{packet: testH264PFrame(raw + 100_000)}
			}
			history.results <- inputResult{packet: testAAC(0)}
			if !resumes {
				history.results <- inputResult{err: io.EOF}
			}
			live.results <- inputResult{err: io.ErrUnexpectedEOF}
			select {
			case s := <-request:
				if s.preRecord != 25 {
					t.Fatalf("history length: %d", s.preRecord)
				}
			case <-time.After(time.Second):
				t.Fatal("no refetch request")
			}
			if !resumes {
				if scenario == "cancel" {
					if err := producer.Stop(); err != nil {
						t.Fatal(err)
					}
					<-canceled
				} else if err := <-done; !errors.Is(err, io.ErrUnexpectedEOF) || !history.aborted.Load() {
					t.Fatalf("fallback did not preserve failure and close history: %v", err)
				}
				if len(video)+len(audio) != 0 {
					t.Fatal("unverified history reached a receiver")
				}
				return
			}
			for _, step := range nextSteps {
				v := get(video)
				if v.Timestamp-lastVideo.Timestamp != step*9/100 {
					t.Fatal("handoff repeated overlap or changed the original video clock")
				}
				lastVideo = v
			}
			a := get(audio)
			if a.Timestamp-lastAudio.Timestamp != 1024 ||
				a.SequenceNumber != lastAudio.SequenceNumber+1 || a.SSRC != lastAudio.SSRC ||
				len(video)+len(audio) != 0 {
				t.Fatal("handoff repeated overlap or changed the original audio clock")
			}
			if scenario == "cadence drain skip" {
				history.results <- inputResult{packet: testH264PFrame(raw + 100_000)}
				if err := <-done; !errors.Is(err, errRefetchContinuity) || !history.aborted.Load() || len(video)+len(audio) != 0 {
					t.Fatalf("later P skip crossed the recovery guard: %v", err)
				}
				return
			}
			if scenario == "cadence resume" {
				// An early I occupies a video slot, and AAC leaves the phase unchanged.
				history.results <- inputResult{packet: testH264PFrame(raw + 50_000)}
				history.results <- inputResult{packet: testAAC(0)}
				history.results <- inputResult{packet: testH264Keyframe(raw + 70_000)}
				history.results <- inputResult{packet: testH264PFrame(raw + 170_000)}
				for _, want := range []uint32{4500, 6300, 15300} {
					if got := get(video).Timestamp - lastVideo.Timestamp; got != want {
						t.Fatalf("post-splice video clock delta: %d, want %d", got, want)
					}
				}
				get(audio)
			}
			history.results <- inputResult{err: io.EOF}
			if err := <-done; !errors.Is(err, io.EOF) || !history.aborted.Load() || len(request) != 0 {
				t.Fatalf("second failure did not use ordinary reconnect: %v", err)
			}
		})
	}
}

func TestRefetchCadence(t *testing.T) {
	for _, test := range []struct {
		name  string
		steps []uint32
		want  []uint32
	}{
		{"wrapped", []uint32{49_984, 50_022, 100_012, 49_986, 50_014, 100_013, 49_994}, []uint32{50_014, 100_013, 49_994}},
		{"one and a half cycles", []uint32{50_000, 100_000, 50_000}, []uint32{50_000}},
		{"constant with jitter", []uint32{50_000, 50_500, 50_000, 50_500}, []uint32{50_500}},
		{"inconsistent prefix", []uint32{140_000, 50_000, 100_000, 50_000, 100_000}, []uint32{100_000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var history refetchHistory
			// Start near the timestamp wrap and put an older discontinuity in the mark ring.
			raw := ^uint32(0) - 100_000
			history.add(refetchMark{track: trackVideo, value: raw - 1_000_000})
			for i := len(test.steps) + 1; i < refetchTail; i++ {
				history.add(refetchMark{track: trackAudio})
			}
			history.add(refetchMark{track: trackVideo, value: raw})
			for _, step := range test.steps {
				raw += step
				history.add(refetchMark{track: trackVideo, value: raw})
				history.step = step
			}
			if got := history.cadence().steps; !slices.Equal(got, test.want) {
				t.Fatalf("cadence %v, want %v", got, test.want)
			}
		})
	}
}

func TestRefetchPlaybackRejectsLaterSkip(t *testing.T) {
	input := &refetchInput{
		packets: []baichuan.MediaPacket{
			testH264PFrame(1_064_000),
			testH264PFrame(1_128_000),
			testH264Keyframe(15_000_000),
		},
		raw: 1_000_000, cadence: refetchCadence{steps: []uint32{64_000}},
		releasedAt: time.Now(),
	}
	start := time.Now()
	for _, want := range []uint32{1_064_000, 1_128_000} {
		packet, err := input.Read(context.Background())
		if err != nil || packet.Timestamp != want {
			t.Fatalf("paced frame: %d, %v", packet.Timestamp, err)
		}
	}
	if time.Since(start) < 60*time.Millisecond {
		t.Fatal("recovery exceeded twice real time")
	}
	if _, err := input.Read(context.Background()); !errors.Is(err, errRefetchContinuity) {
		t.Fatalf("emitted a later timestamp skip: %v", err)
	}
}
