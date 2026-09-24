package jooan

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type fakeSession struct {
	mu         sync.Mutex
	starts     int
	stops      int
	closes     int
	timestamps []uint32
	frames     [][]byte
	channels   []uint32
	ready      bool
	startErr   error
	sendErr    error
	startCh    chan struct{}
}

func (s *fakeSession) AVClientStart(time.Duration) error { return nil }
func (s *fakeSession) HasTwoWayStreaming() bool          { return s.ready }
func (s *fakeSession) AVTwoWayStart(channel uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	s.channels = append(s.channels, channel)
	if s.startCh != nil {
		close(s.startCh)
		s.startCh = nil
	}
	return s.startErr
}
func (s *fakeSession) AVTwoWayStop(channel uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	s.channels = append(s.channels, channel)
	return nil
}
func (s *fakeSession) AVSendAudioDataTwoWay(_ byte, payload []byte, timestamp, _ uint32, _ uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timestamps = append(s.timestamps, timestamp)
	s.frames = append(s.frames, append([]byte(nil), payload...))
	return s.sendErr
}
func (s *fakeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++
	return nil
}

func testProducer(t *testing.T, session *fakeSession) *Producer {
	t.Helper()
	p, err := newProducerWithSession(Config{Host: "192.0.2.10", UID: "synthetic", Username: "admin", Password: "secret"}, session)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProducerFramesExactSizeAndPreservesLeftovers(t *testing.T) {
	session := &fakeSession{ready: true}
	p := testProducer(t, session)
	media := p.Medias[0]
	track := core.NewReceiver(media, media.Codecs[0])
	if err := p.AddTrack(media, media.Codecs[0], track); err != nil {
		t.Fatal(err)
	}

	track.WriteRTP(&rtp.Packet{Payload: make([]byte, 639)})
	track.WriteRTP(&rtp.Packet{Payload: []byte{7, 8}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		ready := len(session.frames) == 1
		session.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.frames) != 1 || len(session.frames[0]) != AudioFrameBytes {
		t.Fatalf("frames = %d, size = %d", len(session.frames), len(session.frames[0]))
	}
	if session.frames[0][639] != 7 {
		t.Fatalf("first frame did not preserve payload order")
	}
	if len(p.frameBuf) != 1 {
		t.Fatalf("leftover = %d, want 1", len(p.frameBuf))
	}
	if session.timestamps[0] != 0 {
		t.Fatalf("timestamp = %d, want 0", session.timestamps[0])
	}
}

func TestProducerAdvertisesOnlyPCMU16KMonoSendonly(t *testing.T) {
	session := &fakeSession{ready: true}
	p := testProducer(t, session)
	if len(p.Medias) != 1 {
		t.Fatalf("media count = %d, want 1", len(p.Medias))
	}
	media := p.Medias[0]
	if media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly || len(media.Codecs) != 1 {
		t.Fatalf("media = %+v", media)
	}
	codec := media.Codecs[0]
	if codec.Name != core.CodecPCMU || codec.ClockRate != AudioSampleRate || codec.Channels != AudioChannels {
		t.Fatalf("codec = %+v", codec)
	}
}

func TestConfigJSONDoesNotExposeCredentials(t *testing.T) {
	data, err := json.Marshal(Config{
		Host: "192.0.2.10", UID: "synthetic-uid", Username: "synthetic-user", Password: "synthetic-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-uid") ||
		strings.Contains(string(data), "synthetic-user") ||
		strings.Contains(string(data), "synthetic-secret") {
		t.Fatalf("config JSON exposed credentials: %s", data)
	}
}

func TestProducerSpeakerLifecycleIsSerializedAndIdempotent(t *testing.T) {
	session := &fakeSession{ready: true}
	p := testProducer(t, session)
	if err := p.AddTrack(p.Medias[0], p.Medias[0].Codecs[0], core.NewReceiver(p.Medias[0], p.Medias[0].Codecs[0])); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.starts != 1 || session.stops != 1 || session.closes != 1 {
		t.Fatalf("lifecycle = starts %d, stops %d, closes %d", session.starts, session.stops, session.closes)
	}
	if len(session.channels) != 2 || session.channels[0] != SpeakerChannel || session.channels[1] != SpeakerChannel {
		t.Fatalf("channels = %v, want [%d %d]", session.channels, SpeakerChannel, SpeakerChannel)
	}
}

func TestProducerRejectsSecondSenderAndIgnoresAudioBeforeStart(t *testing.T) {
	session := &fakeSession{ready: true}
	p := testProducer(t, session)
	p.write(make([]byte, AudioFrameBytes))
	session.mu.Lock()
	if len(session.frames) != 0 {
		t.Fatal("audio was sent before speaker start")
	}
	session.mu.Unlock()

	media := p.Medias[0]
	track := core.NewReceiver(media, media.Codecs[0])
	if err := p.AddTrack(media, media.Codecs[0], track); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])); err == nil {
		t.Fatal("second sender attachment succeeded")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.starts != 1 {
		t.Fatalf("speaker starts = %d, want 1", session.starts)
	}
}

func TestProducerUsesConfiguredSpeakerChannel(t *testing.T) {
	session := &fakeSession{ready: true}
	p, err := newProducerWithSession(Config{
		Host: "192.0.2.10", UID: "synthetic", Username: "admin", Password: "secret",
		SpeakerChannel: 3,
	}, session)
	if err != nil {
		t.Fatal(err)
	}
	media := p.Medias[0]
	if err := p.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.channels) != 2 || session.channels[0] != 3 || session.channels[1] != 3 {
		t.Fatalf("channels = %v, want [3 3]", session.channels)
	}
}

func TestProducerSendFailureStopsSpeakerAndTransport(t *testing.T) {
	session := &fakeSession{ready: true, sendErr: errors.New("synthetic send failure")}
	p := testProducer(t, session)
	media := p.Medias[0]
	if err := p.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])); err != nil {
		t.Fatal(err)
	}
	writeDone := make(chan struct{})
	go func() {
		p.write(make([]byte, AudioFrameBytes))
		close(writeDone)
	}()
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("send failure cleanup deadlocked")
	}
	if err := p.Start(); err == nil || !strings.Contains(err.Error(), "synthetic send failure") {
		t.Fatalf("Start error = %v", err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.stops != 1 || session.closes != 1 {
		t.Fatalf("cleanup = stops %d, closes %d", session.stops, session.closes)
	}
}
func TestProducerTimestampsAdvanceMonotonically(t *testing.T) {
	session := &fakeSession{ready: true}
	p := testProducer(t, session)
	media := p.Medias[0]
	if err := p.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])); err != nil {
		t.Fatal(err)
	}
	p.write(make([]byte, AudioFrameBytes*3))
	session.mu.Lock()
	defer session.mu.Unlock()
	want := []uint32{0, 40, 80}
	if len(session.timestamps) != len(want) {
		t.Fatalf("timestamps = %v, want %v", session.timestamps, want)
	}
	for i := range want {
		if session.timestamps[i] != want[i] {
			t.Fatalf("timestamps = %v, want %v", session.timestamps, want)
		}
	}
}

func TestProducerRejectsMissingTwoWayCapability(t *testing.T) {
	_, err := newProducerWithSession(Config{}, &fakeSession{})
	if !errors.Is(err, ErrNoTwoWay) {
		t.Fatalf("error = %v, want %v", err, ErrNoTwoWay)
	}
}
