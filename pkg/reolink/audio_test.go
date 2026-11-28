package reolink

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/pion/rtp"
)

func TestAACRTPPreservesPayloadAndTimestamps(t *testing.T) {
	packet := testAAC(0)
	original := append([]byte(nil), packet.Data...)
	m := mediaPipeline{}
	if err := m.configureAudio(packet); err != nil {
		t.Fatal(err)
	}
	if !m.audioMedia.Codecs[0].IsRTP() {
		t.Fatal("AAC is advertised as raw audio")
	}
	m.audioTS, m.audioSeq, m.audioSSRC = 1<<32-512, 65535, 1234
	var output *rtp.Packet
	pay := aac.RTPPay(func(packet *rtp.Packet) { output = packet })
	for i, want := range []uint32{1<<32 - 512, 512, 33536} {
		if i == 2 {
			m.audioTS += 32000 // A replacement epoch must survive parent RTP packetization.
		}
		frame, err := m.frame(packet)
		if err != nil || !frame.valid() {
			t.Fatalf("frame: %v %v", frame, err)
		}
		got := frame.packet
		if got.Version != 2 || !got.Marker || got.SSRC != 1234 ||
			got.SequenceNumber != uint16(65535+i) || got.Timestamp != want || frame.samples != 1024 {
			t.Fatalf("unexpected AAC RTP frame: %+v samples=%d", got.Header, frame.samples)
		}
		if binary.BigEndian.Uint16(got.Payload) != 16 ||
			int(binary.BigEndian.Uint16(got.Payload[2:])>>3) != len(original)-aac.ADTSHeaderLen(original) {
			t.Fatalf("invalid AAC AU header: %x", got.Payload)
		}
		var decoded []byte
		aac.RTPDepay(func(packet *rtp.Packet) { decoded = packet.Payload })(got)
		if !bytes.Equal(decoded, original[aac.ADTSHeaderLen(original):]) || !bytes.Equal(packet.Data, original) {
			t.Fatal("AAC packetization changed the encoded audio or ADTS input")
		}
		pay(got)
		if output != got || output.Timestamp != want {
			t.Fatal("parent AAC payloader replaced the supplied timestamp")
		}
	}
}

func TestAACSampleClockIgnoresVideoProgress(t *testing.T) {
	m := mediaPipeline{sharedClock: true, epoch: mediaEpoch{video: 90000}}
	if err := m.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	m.videoTS = mediaClock{set: true, value: 31 * 90000}
	for _, want := range []uint32{16000, 17024} {
		frame, err := m.frame(testAAC(0))
		if err != nil || !frame.valid() || frame.packet.Timestamp != want {
			t.Fatalf("video progress changed sample clock: frame=%v err=%v", frame, err)
		}
	}
}

func TestProfileAACVideoCoherence(t *testing.T) {
	for _, codec := range []string{"H264", "H265"} {
		for _, recover := range []bool{false, true} {
			name := codec + "/failure"
			if recover {
				name = codec + "/catchup"
			}
			t.Run(name, func(t *testing.T) {
				p := newProducer(&Camera{epochs: make(map[ProfileKey]mediaEpoch)}, ProfileKey{}, 1, source{}, nil)
				defer p.cancel()
				m := &p.pipeline
				key := testVideoKeyframe(codec, 1_000_001)
				if err := m.configureVideo(key); err != nil {
					t.Fatal(err)
				}
				if err := m.configureAudio(testAAC(0)); err != nil {
					t.Fatal(err)
				}
				if frame, err := m.frame(key); err != nil || !frame.valid() {
					t.Fatalf("initial keyframe: %v %v", frame, err)
				}
				clock, audio := m.videoTS, m.audioTS
				frame, err := m.frame(testVideoPFrame(codec, 3_000_002))
				if err != nil || !frame.valid() || m.videoResync || m.videoPending.set || m.videoTS == clock {
					t.Fatalf("ahead P frame was not forwarded independently: frame=%v err=%v", frame, err)
				}
				clock = m.videoTS
				frame, err = m.frame(testVideoKeyframe(codec, 3_010_002))
				if err != nil || !frame.valid() || m.videoTS == clock {
					t.Fatalf("first ahead keyframe was not forwarded: frame=%v err=%v", frame, err)
				}
				for _, packet := range []baichuan.MediaPacket{
					testVideoKeyframe(codec, 3_010_002), testVideoPFrame(codec, 3_020_002),
				} {
					frame, err = m.frame(packet)
					if err != nil || !frame.valid() {
						t.Fatalf("ahead video was not forwarded: frame=%v err=%v", frame, err)
					}
				}
				if recover {
					for i := 0; i < 40; i++ {
						frame, err = m.frame(testAAC(0))
						if err != nil || !frame.valid() || frame.packet.Timestamp != uint32(audio+uint64(i)*1024) {
							t.Fatalf("AAC sample clock: %v %v", frame, err)
						}
					}
				}
				beforeConfirm := m.videoTS
				frame, err = m.frame(testVideoKeyframe(codec, 3_050_002))
				if recover && (err != nil || !frame.valid() || m.videoResync || m.videoPending.set || m.videoTS == beforeConfirm) {
					t.Fatalf("AAC catchup did not clear the gap: %v %v", frame, err)
				}
				if !recover && (err == nil || !frame.valid() || m.videoTS == clock || m.audioTS != audio ||
					!strings.Contains(err.Error(), "video ahead of AAC")) {
					t.Fatalf("distinct full keyframe did not forward then fail: %v %v", frame, err)
				}
			})
		}
	}
}

func TestAACGapKeepsLatestVideoEvidenceAcrossInvalidVideo(t *testing.T) {
	m := mediaPipeline{sharedClock: true, epoch: mediaEpoch{video: 90000}}
	if err := m.configureVideo(testH264Keyframe(1_000_001)); err != nil {
		t.Fatal(err)
	}
	if err := m.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	if frame, err := m.frame(testH264Keyframe(1_000_001)); err != nil || !frame.valid() {
		t.Fatalf("initial keyframe: frame=%v err=%v", frame, err)
	}
	if frame, err := m.frame(testH264Keyframe(3_000_002)); err != nil || !frame.valid() {
		t.Fatalf("ahead keyframe: frame=%v err=%v", frame, err)
	}
	if frame, err := m.frame(testH264PFrame(5_000_002)); err != nil || !frame.valid() {
		t.Fatalf("latest P frame was not forwarded: frame=%v err=%v", frame, err)
	}
	clock := m.videoTS
	invalid := testH264PFrame(5_025_002)
	invalid.Data = []byte{0, 0, 0, 1, 0xc1, 1}
	if frame, err := m.frame(invalid); err != nil || frame.valid() {
		t.Fatalf("invalid P frame was not suppressed: frame=%v err=%v", frame, err)
	}
	if m.videoTS != clock || !m.videoResync {
		t.Fatalf("invalid video changed accepted clock or validation state: clock=%+v resync=%t", m.videoTS, m.videoResync)
	}
	for i := 0; i < 40; i++ {
		frame, err := m.frame(testAAC(0))
		if err != nil || !frame.valid() || frame.samples != 1024 {
			t.Fatalf("real AAC sample %d: frame=%v err=%v", i, frame, err)
		}
	}
	if m.audioTS != rescaleTimestamp(90000, 90000, 16000)+40*1024 {
		t.Fatalf("AAC clock did not advance by valid sample units: %d", m.audioTS)
	}
	frame, err := m.frame(testH264Keyframe(5_050_002))
	if err == nil || !frame.valid() || !strings.Contains(err.Error(), "video ahead of AAC") {
		t.Fatalf("AAC progress ignored the latest accepted P: frame=%v err=%v", frame, err)
	}

	m = mediaPipeline{sharedClock: true, epoch: mediaEpoch{video: 90000}}
	if err := m.configureVideo(testH264Keyframe(1_000_001)); err != nil {
		t.Fatal(err)
	}
	if err := m.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	for _, packet := range []baichuan.MediaPacket{
		testH264Keyframe(1_000_001), testH264Keyframe(3_000_002), testH264PFrame(5_000_002),
	} {
		if frame, err = m.frame(packet); err != nil || !frame.valid() {
			t.Fatalf("pre-catch-up video: frame=%v err=%v", frame, err)
		}
	}
	for i := 0; i < 48; i++ {
		if frame, err = m.frame(testAAC(0)); err != nil || !frame.valid() || frame.samples != 1024 {
			t.Fatalf("catch-up AAC sample %d: frame=%v err=%v", i, frame, err)
		}
	}
	bareI := func(timestamp uint32) baichuan.MediaPacket {
		return baichuan.MediaPacket{Kind: baichuan.MediaVideoI, Codec: "H264", Timestamp: timestamp,
			Data: []byte{0, 0, 0, 1, 0x65, 0}}
	}
	if frame, err = m.frame(bareI(5_050_002)); err != nil || !frame.valid() {
		t.Fatalf("AAC catch-up did not clear before a new bare I: frame=%v err=%v", frame, err)
	}
	if frame, err = m.frame(bareI(8_050_002)); err != nil || !frame.valid() {
		t.Fatalf("new ahead bare I was not accepted: frame=%v err=%v", frame, err)
	}
	if frame, err = m.frame(bareI(8_100_002)); err == nil || !frame.valid() || !strings.Contains(err.Error(), "video ahead of AAC") {
		t.Fatalf("new persistent gap did not confirm on a distinct bare I: frame=%v err=%v", frame, err)
	}
}
