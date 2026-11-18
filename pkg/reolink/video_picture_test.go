package reolink

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
)

func TestH265KeyframePictures(t *testing.T) {
	// These independently authored headers exercise picture boundaries, not decoding.
	idr := []byte{0x26, 1, 0x80}
	for _, test := range []struct {
		name  string
		nalus [][]byte
		valid bool
	}{
		{"single", [][]byte{idr}, true},
		{"multislice", [][]byte{idr, {0x26, 1, 0x40}}, true},
		{"dependent slice", [][]byte{idr, {0x26, 1, 0x60}}, true},
		{"other layer", [][]byte{idr, {0x26, 9, 0x80}}, true},
		{"layer high bit", [][]byte{idr, {0x27, 1, 0x80}}, true},
		{"other layers first", [][]byte{{0x26, 9, 0x80}, {0x27, 1, 0x80}, idr}, true},
		{"non VCL", [][]byte{idr, {0x4e, 1, 0x80}}, true},
		{"compound", [][]byte{idr, {2, 1, 0x80}, {2, 1, 0x80}, {2, 1, 0x80}}, false},
		{"second IDR", [][]byte{idr, idr}, false},
		{"first slice before IDR", [][]byte{{2, 1, 0x80}, idr}, false},
		{"truncated IDR", [][]byte{{0x26, 1}}, false},
		{"truncated later slice", [][]byte{idr, {2, 1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			nalus := [][]byte{{0x40, 1}, {0x42, 1}, {0x44, 1}}
			nalus = append(nalus, test.nalus...)
			payload := annexb.EncodeToAVCC(testH265KeyframeData(nalus...))
			if got := validVideoConfig("H265", payload); got != test.valid {
				t.Fatalf("accepted=%t, want %t", got, test.valid)
			}
		})
	}
	// Picture counting is confined to H265 keyframes.
	if _, err := inspectVideoPayload("H265", annexb.EncodeToAVCC(testH265KeyframeData(
		[]byte{2, 1, 0x80}, []byte{2, 1, 0x80},
	))); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectVideoPayload("H264", annexb.EncodeToAVCC(h264AccessUnit(
		[]byte{0x65, 0x80}, []byte{0x65, 0x80},
	))); err != nil {
		t.Fatal(err)
	}
}

func TestH265CompoundRecoveryPreservesState(t *testing.T) {
	for _, resync := range []bool{false, true} {
		for _, changed := range []bool{false, true} {
			m := mediaPipeline{sharedClock: true}
			if err := m.configureVideo(testVideoKeyframe("H265", 0)); err != nil {
				t.Fatal(err)
			}
			if err := m.configureAudio(testAAC(0)); err != nil {
				t.Fatal(err)
			}
			if frame, err := m.videoFrame(testVideoKeyframe("H265", 1_000_000)); err != nil || !frame.valid() {
				t.Fatalf("initial frame: %v %v", frame, err)
			}
			clock, params, config, audio := m.videoTS, m.videoParams, m.videoMedia.Codecs[0].FmtpLine, m.audioTS
			m.videoResync = resync
			m.videoCandidate = "unconfirmed"
			m.videoPending = videoClockCandidate{first: 2_000_000, last: 2_000_000, set: true}
			packet := testH265CompoundFrame(3_000_000)
			if changed {
				packet.Data[5] ^= 8
			}
			if frame, err := m.videoFrame(packet); err != nil || frame.valid() {
				t.Fatalf("compound recovery accepted: resync=%t changed=%t frame=%v err=%v", resync, changed, frame, err)
			}
			if m.videoTS != clock || m.audioTS != audio || m.videoParams != params ||
				m.videoMedia.Codecs[0].FmtpLine != config || !m.videoResync || m.videoCandidate != "" || m.videoPending.set {
				t.Fatal("compound recovery changed committed state or retained candidates")
			}
			if frame, err := m.videoFrame(testVideoPFrame("H265", 3_050_000)); err != nil || frame.valid() {
				t.Fatalf("dependent P escaped recovery: %v %v", frame, err)
			}
			// The coherent-video resync still completes, and an AAC gap does not withhold it.
			frame, err := m.videoFrame(testVideoKeyframe("H265", 3_100_000))
			if err != nil || !frame.valid() || m.videoTS == clock || m.videoResync || m.videoPending.set {
				t.Fatalf("accepted I was withheld by the AAC gap: %v %v", frame, err)
			}
			for range 20 {
				if frame, err := m.audioFrame(testAAC(0)); err != nil || !frame.valid() || frame.samples != 1024 {
					t.Fatalf("AAC stopped during recovery: %v %v", frame, err)
				}
			}
			if m.audioTS != audio+20*1024 {
				t.Fatal("AAC sample clock changed")
			}
			frame, err = m.videoFrame(testVideoKeyframe("H265", 3_200_000))
			if err != nil || !frame.valid() || frame.packet.Timestamp != 198000 || m.videoResync || m.videoPending.set {
				t.Fatalf("coherent recovery lost accepted clock interval: %v %v", frame, err)
			}
			if frame, err := m.videoFrame(testVideoPFrame("H265", 3_250_000)); err != nil || !frame.valid() ||
				frame.packet.Timestamp != 202500 {
				t.Fatalf("P after recovery: %v %v", frame, err)
			}
		}
	}
}

func testH265CompoundFrame(timestamp uint32) baichuan.MediaPacket {
	packet := testVideoKeyframe("H265", timestamp)
	packet.Data = append(packet.Data, testH265KeyframeData(
		[]byte{2, 1, 0x80}, []byte{2, 1, 0x80}, []byte{2, 1, 0x80},
	)...)
	return packet
}
