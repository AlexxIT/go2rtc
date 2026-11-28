package mp4_test

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/pion/rtp"
	goopus "github.com/skrashevich/go-opus"
)

type opusTestProducer struct{ core.Connection }

func (p *opusTestProducer) Start() error { return nil }

type opusTestWriter struct {
	bytes.Buffer
	mu     sync.Mutex
	writes int
	init   chan struct{}
	media  chan struct{}
}

func (w *opusTestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.writes == 1 {
		close(w.init)
	}
	if w.writes == 2 {
		close(w.media)
	}
	return w.Buffer.Write(p)
}

func TestAddConsumerOpusToMP4FLAC(t *testing.T) {
	for _, channels := range []int{1, 2} {
		t.Run(strconv.Itoa(channels), func(t *testing.T) { testOpusToFLAC(t, channels) })
	}
}
func testOpusToFLAC(t *testing.T, channels int) {
	encoder, err := goopus.NewEncoder(goopus.WithChannels(channels))
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1276)
	n, err := encoder.Encode(make([]byte, 1920*channels), data)
	data = data[:max(n, 0)]
	if err != nil {
		t.Fatal(err)
	}
	testToFLAC(t, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: uint8(channels)}, data, mp4.ParseQuery(map[string][]string{"mp4": {"flac"}}), 48000, 960)
}

func TestAACToMP4FLAC(t *testing.T) {
	data, err := os.ReadFile("../audio/testdata/stereo.aac")
	if err != nil {
		t.Fatal(err)
	}
	codec := aac.ADTSToCodec(data)
	codec.PayloadType = core.PayloadTypeRAW
	data = data[7:aac.ReadADTSSize(data)]
	testToFLAC(t, codec, data, mp4.ParseQuery(map[string][]string{"audio": {"pcml"}}), 44100, 1024)
}

func testToFLAC(t *testing.T, codec *core.Codec, data []byte, medias []*core.Media, rate, durationExpected uint32) {
	t.Helper()
	prod := &opusTestProducer{Connection: core.Connection{Medias: []*core.Media{{
		Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{codec},
	}}}}
	cons := mp4.NewConsumer(medias)
	stream := streams.NewStream(nil)
	stream.AddProducer(prod)
	if err := stream.AddConsumer(cons); err != nil {
		t.Fatal(err)
	}
	defer stream.RemoveConsumer(cons)
	if len(cons.Senders) != 1 || cons.Senders[0].Codec.Name != core.CodecFLAC || cons.Senders[0].Codec.ClockRate != rate || cons.Senders[0].Codec.Channels != 1 {
		t.Fatalf("MP4 senders = %v; want FLAC", cons.Senders)
	}
	output := &opusTestWriter{init: make(chan struct{}), media: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := cons.WriteTo(output); done <- err }()
	select {
	case <-output.init:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out writing MP4 init")
	}
	prod.Receivers[0].WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2}, Payload: data})
	select {
	case <-output.media:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FLAC media payload")
	}
	cons.Stop()
	cons.Senders[0].Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out writing MP4")
	}
	if cons.Send == 0 {
		t.Fatal("Opus to FLAC conversion produced no MP4 media payload")
	}
	atoms, err := iso.DecodeAtoms(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var duration uint32
	for _, atom := range atoms {
		if audio, ok := atom.(*iso.AtomAudio); ok && (audio.Channels != 1 || audio.SampleRate != rate) {
			t.Fatalf("FLAC metadata: %+v", audio)
		}
		if header, ok := atom.(*iso.AtomTfhd); ok {
			duration = header.SampleDuration
		}
	}
	if duration != durationExpected {
		t.Fatalf("FLAC fragment duration = %d, want %d samples", duration, durationExpected)
	}
}
