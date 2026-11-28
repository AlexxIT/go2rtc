package reolink

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

func TestRegistrySetupFailureReleasesSession(t *testing.T) {
	for _, stage := range []string{"open", "probe"} {
		t.Run(stage, func(t *testing.T) {
			cause, closeErr := errors.New("setup failure"), errors.New("abort failure")
			var camera *Camera
			input := &fakeProfileInput{results: make(chan inputResult, 2), done: make(chan struct{}), closeErr: closeErr}
			input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
			input.results <- inputResult{err: cause}
			registry := testRegistry(func(_ context.Context, c *Camera, _ source) (profileInput, error) {
				camera = c
				if stage == "open" {
					return nil, cause
				}
				return input, nil
			})
			if _, err := registry.Dial("reolink://u:p@camera/main?backchannel=0"); !errors.Is(err, cause) {
				t.Fatalf("lost setup cause: %v", err)
			}
			if stage == "probe" && !input.aborted.Load() {
				t.Fatal("failed probe input was not aborted before Dial returned")
			}
			registry.mu.Lock()
			defer registry.mu.Unlock()
			if len(registry.cameras) != 0 || len(camera.profiles) != 0 || camera.refs != 0 ||
				camera.mainSessions != 0 || camera.otherSessions != 0 {
				t.Fatalf("setup retained camera/session: %+v", camera)
			}
			key := ProfileKey{Stream: baichuan.StreamMain}
			if users := camera.epochs[key].aac.users; users != 0 {
				t.Fatalf("setup retained %d AAC epoch users", users)
			}
		})
	}
}

func TestProducerFailureBeforeStartRetainsCause(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://u:p@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Stop()
	opener.input(0).results <- inputResult{err: io.ErrUnexpectedEOF}
	waitFor(t, func() bool {
		select {
		case <-producer.done:
			return true
		default:
			return false
		}
	})
	if err = producer.Start(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Start lost pre-start terminal cause: %v", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	camera := producer.camera
	if camera.mainSessions != 0 || camera.refs != 1 || len(camera.profiles) != 0 ||
		camera.epochs[producer.key].aac.users != 0 {
		t.Fatal("terminal worker did not release its session while retaining its owner")
	}
}

func TestProducerReadyPublishesConnection(t *testing.T) {
	registry := testRegistry((&fakeProfileOpener{}).open)
	s, err := parseURL("reolink://u:p@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	producer := registry.acquire(registry.cameraKey(s), s)
	defer registry.abort(producer)
	if err = producer.setup(); err != nil {
		t.Fatal(err)
	}
	if len(producer.GetMedias()) != 2 || len(producer.Receivers) != 2 {
		t.Fatal("ready published before core media/receiver initialization")
	}
	for i, media := range producer.GetMedias() {
		receiver, trackErr := producer.GetTrack(media, media.Codecs[0])
		if trackErr != nil || receiver != producer.Receivers[i] {
			t.Fatalf("ready track %d differs from its core receiver: %v", i, trackErr)
		}
		if receiver != producer.video && receiver != producer.audio {
			t.Fatal("media writer points outside core receiver collection")
		}
	}
	if len(producer.Receivers) != 2 {
		t.Fatal("GetTrack created a duplicate receiver after ready")
	}
}

func TestRegistryOptionalTalkFailureKeepsMedia(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, &baichuan.StatusError{Command: 10, Code: 500},
		errors.New("sentinel-user sentinel-pass sentinel-camera-xml-private")} {
		t.Run(errorClass(cause), func(t *testing.T) {
			input := newFakeProfileInput()
			input.talkErr = cause
			registry := testRegistry(func(context.Context, *Camera, source) (profileInput, error) { return input, nil })
			var logs bytes.Buffer
			registry.log = zerolog.New(&logs)
			producer, err := registry.Dial("reolink://sentinel-user:sentinel-pass@camera/main?audio=0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = producer.Stop() })
			if producer.talk != nil || len(producer.Medias) != 1 || input.talks.Load() != 1 || input.closed.Load() {
				t.Fatal("optional talk failure changed media ownership or advertised talk")
			}
			text := logs.String()
			for _, secret := range []string{"sentinel-user", "sentinel-pass", "sentinel-camera-xml-private", "reolink://"} {
				if strings.Contains(text, secret) {
					t.Fatalf("probe warning exposed %s", secret)
				}
			}
			for _, field := range []string{`"level":"warn"`, `"camera":"camera:9000"`, `"channel":0`,
				`"profile":"` + string(baichuan.StreamMain) + `"`, `"generation":1`, `"error_class":"` + errorClass(cause) + `"`} {
				if !strings.Contains(text, field) {
					t.Fatalf("missing warning field %s: %s", field, text)
				}
			}
			if strings.Count(text, `"level":"warn"`) != 1 {
				t.Fatalf("expected one warning: %s", text)
			}
			packets := make(chan *rtp.Packet, 2)
			sender := core.NewSender(producer.video.Media, producer.video.Codec)
			sender.Handler = func(packet *rtp.Packet) { packets <- packet }
			sender.HandleRTP(producer.video)
			defer sender.Close()
			go func() { _ = producer.Start() }()
			var timestamp uint32
			for i := range 2 {
				select {
				case packet := <-packets:
					if i == 1 && packet.Timestamp == timestamp {
						t.Fatal("media did not advance after replay")
					}
					timestamp = packet.Timestamp
				case <-time.After(time.Second):
					t.Fatalf("missing video packet %d", i)
				}
				if i == 0 {
					input.results <- inputResult{packet: testH264PFrame(1_050_000)}
				}
			}
		})
	}
}

func TestRegistryTalkProbeFailureReleasesSession(t *testing.T) {
	for _, mode := range []string{"talk-only", "terminal", "canceled", "input-closed"} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New("talk probe failure")
			want := cause
			input := newFakeProfileInput()
			var camera *Camera
			registry := testRegistry(func(_ context.Context, c *Camera, _ source) (profileInput, error) {
				camera = c
				input.probeTalk = func(context.Context) error {
					c.registry.mu.Lock()
					var profile *Producer
					for p := range c.profiles {
						profile = p
					}
					c.registry.mu.Unlock()
					switch mode {
					case "terminal":
						profile.beginTerminal(cause)
					case "canceled":
						want = context.Canceled
						profile.cancel()
					case "input-closed":
						input.inputErr = cause
					}
					if mode == "talk-only" {
						return cause
					}
					return context.DeadlineExceeded
				}
				return input, nil
			})
			url := "reolink://u:p@camera/main"
			if mode == "talk-only" {
				url += "?video=0&audio=0"
			}
			if producer, err := registry.Dial(url); producer != nil || !errors.Is(err, want) {
				t.Fatalf("Dial returned producer=%v error=%v, want %v", producer, err, want)
			}
			registry.mu.Lock()
			defer registry.mu.Unlock()
			if !input.closed.Load() || camera.refs != 0 || camera.mainSessions != 0 ||
				len(camera.profiles) != 0 || len(registry.cameras) != 0 {
				t.Fatal("failed talk probe retained its input, profile or session")
			}
		})
	}
}
