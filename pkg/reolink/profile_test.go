package reolink

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func deliveryTestProfile() *Profile {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	media.Codecs = []*core.Codec{codec}
	receiver := core.NewReceiver(media, codec)
	return &Profile{video: receiver, receivers: []*core.Receiver{receiver}}
}

func TestProducerDiagnosticsAreRedacted(t *testing.T) {
	opener := &fakeProfileOpener{discovery: discoverySnapshot{
		device: baichuan.DeviceInfo{
			Type: "E1 Zoom", Model: "E340", Hardware: "IPC_NT14", Firmware: "v3.2.0",
		},
		capabilities: baichuan.Capabilities{
			ObservedChannels: []uint8{0},
		},
		deviceStatus: "available", capabilityStatus: "available",
	}}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://sentinel-user:sentinel-pass@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	producer.Source = "reolink://sentinel-user:sentinel-pass@camera"
	producer.URL = producer.Source
	data, err := json.Marshal(producer)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "sentinel-user") || strings.Contains(text, "sentinel-pass") {
		t.Fatalf("credentials exposed in diagnostics: %s", text)
	}
	for _, expected := range []string{
		`"remote_addr":"camera:9000"`, `"generation":1`, `"main_sessions":1`,
		`"profiles":1`, `"camera_type":"E1 Zoom"`,
		`"camera_model":"E340"`, `"hardware_version":"IPC_NT14"`,
		`"firmware_version":"v3.2.0"`, `"device_info_status":"available"`,
		`"capability_status":"available"`, `"observed_channels":[0]`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in diagnostics: %s", expected, text)
		}
	}
	_ = producer.Stop()
	waitFor(t, opener.input(0).closed.Load)
}

func TestProducerDiagnosticsDuringDelivery(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	input := opener.input(0)
	go func() {
		for i := range 1000 {
			input.results <- inputResult{packet: testH264PFrame(uint32(1_050_000 + i*50_000))}
		}
		input.results <- inputResult{err: io.ErrUnexpectedEOF}
	}()
	for range 1000 {
		if _, err = json.Marshal(producer); err != nil {
			t.Fatal(err)
		}
	}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if !input.aborted.Load() {
		t.Fatal("terminal failure closed input gracefully")
	}
	if class := producer.diagnostics().LastError; class != "eof" {
		t.Fatalf("truncated network frame classified as %q", class)
	}
	if video, audio := producer.profile.videoFrames.Load(), producer.profile.audioSamples.Load(); video != 1001 || audio == 0 {
		t.Fatalf("unexpected liveness state: video=%d audio_samples=%d", video, audio)
	}
	_ = producer.Stop()
}

func TestProducerResyncsWithoutRestartAfterInvalidVideo(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	waitFor(t, func() bool { return producer.profile.startCtx.Err() != nil })
	input := opener.input(0)
	frames := producer.profile.videoFrames.Load()
	input.results <- inputResult{packet: baichuan.MediaPacket{
		Kind: baichuan.MediaVideoP, Codec: "H264", Timestamp: 1_050_000,
		Data: []byte{0, 0, 0, 1, 0xc1, 1},
	}}
	input.results <- inputResult{packet: testH264PFrame(1_100_000)}
	input.results <- inputResult{packet: testH264Keyframe(1_150_000)}
	waitFor(t, func() bool { return producer.profile.videoFrames.Load() == frames+1 })
	select {
	case err = <-done:
		t.Fatalf("video repair stopped producer: %v", err)
	default:
	}
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileWritesFrameDirectlyToReceiver(t *testing.T) {
	profile := deliveryTestProfile()
	var received *rtp.Packet
	(&core.Node{Input: func(packet *rtp.Packet) {
		received = packet
	}}).WithParent(&profile.video.Node)
	frame := mediaFrame{track: trackVideo, packet: &rtp.Packet{
		Header: rtp.Header{Timestamp: 7}, Payload: []byte{1, 2, 3},
	}}
	profile.writeFrame(frame)
	if received != frame.packet {
		t.Fatal("profile copied the RTP packet")
	}
}

func TestProducerStartReplaysProbeAfterTrackAttachment(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	packets := make(chan *rtp.Packet, 1)
	(&core.Node{Input: func(packet *rtp.Packet) { packets <- packet }}).WithParent(&producer.Receivers[0].Node)
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	select {
	case packet := <-packets:
		if packet.Timestamp == 0 || len(packet.Payload) == 0 {
			t.Fatalf("unexpected probe frame: timestamp=%d size=%d", packet.Timestamp, len(packet.Payload))
		}
	case <-time.After(time.Second):
		t.Fatal("probe frame was not replayed after start")
	}
	select {
	case err = <-done:
		t.Fatalf("producer stopped before cancellation: %v", err)
	default:
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected stop error: %v", err)
	}
	input := opener.input(0)
	if !input.closed.Load() {
		t.Fatal("producer stop returned before its input closed")
	}
	if input.aborted.Load() {
		t.Fatal("producer stop aborted input")
	}
}

func TestProfileProbeSkipsInvalidInitialVideoKeyframe(t *testing.T) {
	input := &fakeProfileInput{results: make(chan inputResult, 3), done: make(chan struct{})}
	invalid := testVideoKeyframe("H265", 1_000_000)
	invalid.Data[4] |= 0x80
	input.results <- inputResult{packet: invalid}
	input.results <- inputResult{packet: testVideoKeyframe("H265", 1_050_000)}
	input.results <- inputResult{packet: testAAC(1_050_000)}

	profile := &Profile{ctx: context.Background()}
	probe, err := profile.probe(input)
	if err != nil {
		t.Fatal(err)
	}
	if profile.pipeline.videoMedia == nil || profile.pipeline.videoCodec != "H265" {
		t.Fatal("probe did not configure the valid replacement keyframe")
	}
	if len(probe.packets) != 2 || probe.packets[0].Kind != baichuan.MediaVideoI {
		t.Fatalf("unexpected retained probe: packets=%d", len(probe.packets))
	}
}

func TestProfileProbeRequiresEnabledAudio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	input := &fakeProfileInput{results: make(chan inputResult, 1), done: make(chan struct{})}
	input.results <- inputResult{packet: testH264Keyframe(1_000_000)}
	profile := &Profile{ctx: ctx}
	if _, err := profile.probe(input); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("missing audio did not fail probe: %v", err)
	}
}

func TestProducerRetainsReceiversForReconnect(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	receiver := producer.Receivers[0]
	recording := make(chan *rtp.Packet, 2)
	live := make(chan *rtp.Packet, 2)
	(&core.Node{Input: func(packet *rtp.Packet) { recording <- packet }}).WithParent(&receiver.Node)
	(&core.Node{Input: func(packet *rtp.Packet) { live <- packet }}).WithParent(&receiver.Node)
	wantPacket := func(name string, packets <-chan *rtp.Packet) *rtp.Packet {
		t.Helper()
		select {
		case packet := <-packets:
			return packet
		case <-time.After(time.Second):
			t.Fatalf("%s consumer did not receive a packet", name)
			return nil
		}
	}
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	wantPacket("recording", recording)
	wantPacket("live", live)
	opener.input(0).results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected terminal error: %v", err)
	}

	replacement := core.NewReceiver(receiver.Media, receiver.Codec)
	receiver.Replace(replacement)
	packet := &rtp.Packet{Header: rtp.Header{Timestamp: 42}, Payload: []byte{1}}
	replacement.WriteRTP(packet)
	for name, packets := range map[string]<-chan *rtp.Packet{"recording": recording, "live": live} {
		if got := wantPacket(name, packets); got != packet {
			t.Fatalf("replacement copied the RTP packet for %s", name)
		}
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProducerReconnectUsesForwardTimestampEpoch(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	first, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	videoReceiver := first.Receivers[0]
	audioReceiver := first.Receivers[1]
	videoPackets := make(chan *rtp.Packet, 4)
	audioPackets := make(chan *rtp.Packet, 4)
	(&core.Node{Input: func(packet *rtp.Packet) { videoPackets <- packet }}).WithParent(&videoReceiver.Node)
	(&core.Node{Input: func(packet *rtp.Packet) { audioPackets <- packet }}).WithParent(&audioReceiver.Node)
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.Start() }()
	lastVideo := (<-videoPackets).Timestamp
	lastAudio := (<-audioPackets).Timestamp
	opener.input(0).results <- inputResult{packet: testH264PFrame(1_050_000)}
	opener.input(0).results <- inputResult{packet: testAAC(1_050_000)}
	lastVideo = (<-videoPackets).Timestamp
	lastAudio = (<-audioPackets).Timestamp
	opener.input(0).results <- inputResult{err: io.ErrUnexpectedEOF}
	if err = <-firstDone; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected first terminal error: %v", err)
	}

	replacement, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	videoReceiver.Replace(replacement.Receivers[0])
	audioReceiver.Replace(replacement.Receivers[1])
	replacementDone := make(chan error, 1)
	go func() { replacementDone <- replacement.Start() }()
	nextVideo := (<-videoPackets).Timestamp
	nextAudio := (<-audioPackets).Timestamp
	if !timestampAfter(nextVideo, lastVideo) || !timestampAfter(nextAudio, lastAudio) {
		t.Fatalf("replacement timestamps did not advance: video=%d->%d audio=%d->%d",
			lastVideo, nextVideo, lastAudio, nextAudio)
	}
	if err = first.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = replacement.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = <-replacementDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected replacement terminal error: %v", err)
	}
}

func TestProducerStopPropagatesInputCloseError(t *testing.T) {
	opener := &fakeProfileOpener{}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	closeErr := errors.New("sentinel input close")
	opener.input(0).closeErr = closeErr
	if err = producer.Stop(); !errors.Is(err, closeErr) {
		t.Fatalf("input close error was suppressed: %v", err)
	}
	if err = producer.Stop(); !errors.Is(err, closeErr) {
		t.Fatalf("repeated stop lost close error: %v", err)
	}
}

func TestProducerTalkUsesProfileInputAndStopsBeforeProfile(t *testing.T) {
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	opener := &fakeProfileOpener{talk: &format}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?channel=2&backchannel=1")
	if err != nil {
		t.Fatal(err)
	}
	input := opener.input(0)
	opened := make(chan *fakeTalk, 1)
	input.startTalk = func(context.Context) (talkSession, error) {
		talk := &fakeTalk{format: format, blocks: make(chan []byte, 1)}
		talk.closeFn = func() error {
			if producer.profile.ctx.Err() != nil {
				return errors.New("profile canceled before talk stopped")
			}
			return nil
		}
		opened <- talk
		return talk, nil
	}
	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}
	media := &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec},
	}
	track := core.NewReceiver(media, codec)
	defer track.Close()
	if err = producer.AddTrack(media, codec, track); err != nil {
		t.Fatal(err)
	}
	track.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	first := wantTalkOpen(t, opened)
	wantBlock(t, first, format.BytesPerBlock())
	if input.starts.Load() != 1 || input.channel.Load() != 2 {
		t.Fatalf("talk did not use profile input: starts=%d channel=%d", input.starts.Load(), input.channel.Load())
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
	if first.closes != 1 || !input.closed.Load() {
		t.Fatalf("unexpected stop lifecycle: talk_closes=%d input_closed=%t", first.closes, input.closed.Load())
	}
}

func TestProducerFailureStopsProfileBeforeReconnect(t *testing.T) {
	format := baichuan.TalkFormat{SampleRate: 16000, SamplePrecision: 16, SamplesPerBlock: 4}
	opener := &fakeProfileOpener{talk: &format}
	registry := testRegistry(opener.open)
	producer, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=1")
	if err != nil {
		t.Fatal(err)
	}
	opener.input(0).startTalk = func(context.Context) (talkSession, error) {
		return &fakeTalk{
			format: format, blocks: make(chan []byte, 1), writeErr: errors.New("sentinel producer failure"),
		}, nil
	}
	done := make(chan error, 1)
	go func() { done <- producer.Start() }()
	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}
	media := &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec},
	}
	track := core.NewReceiver(media, codec)
	if err = producer.AddTrack(media, codec, track); err != nil {
		t.Fatal(err)
	}
	track.WriteRTP(&rtp.Packet{Payload: []byte{0xd5, 0xd5}})
	select {
	case err = <-done:
		if err == nil || !strings.Contains(err.Error(), "sentinel producer failure") {
			t.Fatalf("unexpected terminal error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("talkback failure did not stop the producer")
	}
	data, err := json.Marshal(producer)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"last_error_class":"protocol"`) {
		t.Fatalf("diagnostics lost producer failure class: %s", data)
	}
	replacement, err := registry.Dial("reolink://admin:secret@camera/main?backchannel=1")
	if err != nil {
		t.Fatalf("replacement rejected after terminal failure: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = replacement.Stop(); err != nil {
		t.Fatal(err)
	}
}
