package reolink

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestCompoundVideoRecoversAudio(t *testing.T) {
	testCompoundVideoRecoversAudio(t, (*Producer).writePacket)
}

func TestAACGapConfirmationWritesBeforeTerminalAndSavesEpoch(t *testing.T) {
	registry := testRegistry(nil)
	s := source{stream: baichuan.StreamMain, remote: "camera"}
	profile := registry.acquire(cameraKey{endpoint: "camera"}, s)
	camera, key := profile.camera, profile.key
	t.Cleanup(func() {
		profile.cancel()
		profile.closeReceivers()
		registry.release(profile)
	})

	const (
		wireWidth = uint64(1) << 32
		wireMask  = wireWidth - 1
	)
	start := (nowTimestamp(90000) &^ wireMask) + wireWidth - 4500
	profile.pipeline = mediaPipeline{epoch: mediaEpoch{video: start}, sharedClock: true}
	profile.pipeline.videoResyncs = &profile.videoResyncs
	if err := profile.pipeline.configureVideo(testH264Keyframe(1_000_000)); err != nil {
		t.Fatal(err)
	}
	if err := profile.pipeline.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	profile.initReceivers()
	var received []*rtp.Packet
	(&core.Node{Input: func(packet *rtp.Packet) { received = append(received, packet) }}).WithParent(&profile.video.Node)
	bareI := func(timestamp uint32) baichuan.MediaPacket {
		return baichuan.MediaPacket{
			Kind: baichuan.MediaVideoI, Codec: "H264", Timestamp: timestamp,
			Data: []byte{0, 0, 0, 1, 0x65, 0},
		}
	}
	for _, packet := range []baichuan.MediaPacket{testH264Keyframe(1_000_000), bareI(3_000_000)} {
		if err := profile.writePacket(packet); err != nil {
			t.Fatalf("pre-confirmation packet: %v", err)
		}
	}
	candidateClock := profile.pipeline.videoTS.value
	if err := profile.writePacket(bareI(3_000_000)); err != nil {
		t.Fatalf("duplicate I timestamp: %v", err)
	}
	if profile.pipeline.videoTS.value != candidateClock {
		t.Fatal("duplicate full-width I timestamp changed the accepted video clock")
	}
	if err := profile.writePacket(testH264PFrame(3_025_000)); err != nil {
		t.Fatalf("intervening P frame: %v", err)
	}
	confirmErr := profile.writePacket(bareI(3_050_000))
	if confirmErr == nil || !strings.Contains(confirmErr.Error(), "video ahead of AAC") {
		t.Fatalf("persistent gap did not terminate: %v", confirmErr)
	}
	confirmed := profile.pipeline.videoTS.value
	if len(received) != 5 {
		t.Fatalf("confirmation delivery count = %d, want 5", len(received))
	}
	if profile.videoFrames.Load() != 5 || received[4].Timestamp != uint32(confirmed) || confirmed>>32 == start>>32 {
		t.Fatalf("confirmation was not emitted exactly once across RTP wrap: frames=%d received=%d wire=%d clock=%d",
			profile.videoFrames.Load(), len(received), received[4].Timestamp, confirmed)
	}
	if !profile.beginTerminal(confirmErr) {
		t.Fatal("confirmation error did not make the profile terminal")
	}
	profile.finishSession(nil)
	if epoch := camera.epochs[key]; !epoch.videoSet || epoch.video != confirmed || !epoch.audioSet ||
		epoch.audio != profile.pipeline.audioTS || epoch.audioRate != profile.pipeline.audioRate {
		t.Fatalf("terminal epoch did not save emitted video and AAC clocks: %+v want video=%d audio=%d rate=%d",
			epoch, confirmed, profile.pipeline.audioTS, profile.pipeline.audioRate)
	}
	replacement := registry.acquire(camera.key, s)
	t.Cleanup(func() {
		registry.release(replacement)
		replacement.finishSession(nil)
		replacement.closeReceivers()
		replacement.cancel()
	})
	if replacement.pipeline.epoch.video != confirmed+1 {
		t.Fatalf("replacement epoch did not advance past emitted confirmation: %d <= %d",
			replacement.pipeline.epoch.video, confirmed)
	}
}

func testCompoundVideoRecoversAudio(t *testing.T, write func(*Producer, baichuan.MediaPacket) error) {
	t.Helper()
	p := &Producer{}
	if err := p.pipeline.configureVideo(testVideoKeyframe("H265", 0)); err != nil {
		t.Fatal(err)
	}
	if err := p.pipeline.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	p.initReceivers()
	t.Cleanup(p.closeReceivers)
	var received []*rtp.Packet
	(&core.Node{Input: func(packet *rtp.Packet) { received = append(received, packet) }}).WithParent(&p.audio.Node)
	sequence, ssrc := p.pipeline.audioSeq, p.pipeline.audioSSRC
	if err := write(p, testVideoKeyframe("H265", 1_000_000)); err != nil {
		t.Fatal(err)
	}
	clock, audio := p.pipeline.videoTS, p.pipeline.audioTS
	packet := testH265CompoundFrame(1_050_000)
	for range 2 {
		data := testAAC(0).Data
		header := make([]byte, 8)
		binary.LittleEndian.PutUint32(header, 0x62773530)
		binary.LittleEndian.PutUint16(header[4:], uint16(len(data)))
		binary.LittleEndian.PutUint16(header[6:], uint16(len(data)))
		packet.Data = append(packet.Data, header...)
		packet.Data = append(packet.Data, data...)
	}
	// Corroborate the final AAC with a real embedded video header/prefix.
	header := make([]byte, 24)
	binary.LittleEndian.PutUint32(header, 0x63643130)
	copy(header[4:], "H265")
	data := testVideoPFrame("H265", 0).Data
	binary.LittleEndian.PutUint32(header[8:], uint32(len(data)))
	packet.Data = append(packet.Data, header...)
	packet.Data = append(packet.Data, data...)
	if err := write(p, packet); err != nil {
		t.Fatal(err)
	}
	if p.audioSamples.Load() != 2048 || p.pipeline.audioTS != audio+2048 {
		t.Fatalf("recovered samples=%d clock advance=%d", p.audioSamples.Load(), p.pipeline.audioTS-audio)
	}
	if p.pipeline.videoTS != clock || !p.pipeline.videoResync || p.videoFrames.Load() != 1 {
		t.Fatal("malformed video advanced or released video recovery")
	}
	if err := write(p, testVideoPFrame("H265", 1_100_000)); err != nil || p.videoFrames.Load() != 1 {
		t.Fatalf("dependent picture escaped recovery: %v", err)
	}
	if err := write(p, testVideoKeyframe("H265", 1_150_000)); err != nil || p.videoFrames.Load() != 2 {
		t.Fatalf("coherent keyframe failed recovery: %v", err)
	}
	if err := write(p, baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: testAAC(0).Data}); err != nil || p.audioSamples.Load() != 3072 {
		t.Fatalf("ordinary audio after salvage: %v samples=%d", err, p.audioSamples.Load())
	}
	if len(received) != 3 {
		t.Fatalf("audio deliveries=%d", len(received))
	}
	for i, packet := range received {
		if packet.Version != 2 || !packet.Marker || packet.PayloadType != 96 || packet.SSRC != ssrc ||
			packet.SequenceNumber != sequence+uint16(i) || packet.Timestamp != uint32(audio)+uint32(i)*1024 ||
			string(packet.Payload) != string([]byte{0, 16, 0, 24, 1, 2, 3}) {
			t.Fatalf("audio RTP %d: %+v", i, packet)
		}
	}
}
