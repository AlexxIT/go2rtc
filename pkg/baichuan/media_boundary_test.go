package baichuan

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

func recoveryPFixture(codec string) []byte {
	nal := []byte{1}
	if codec == "H265" {
		nal = []byte{2, 1}
	}
	payload := append([]byte{0, 0, 0, 1}, nal...)
	b := make([]byte, mediaVideoHeaderSize+len(payload)+int(padding(uint32(len(payload)))))
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], codec)
	binary.LittleEndian.PutUint32(b[8:12], uint32(len(payload)))
	copy(b[mediaVideoHeaderSize:], payload)
	return b
}

func recoveryADPCMFixture() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaADPCM)
	binary.LittleEndian.PutUint16(b[4:6], 6)
	binary.LittleEndian.PutUint16(b[6:8], 6)
	binary.LittleEndian.PutUint16(b[8:10], 0x0100)
	b[10], b[11] = 0x12, 0x34
	return b
}

func recoveryAACFixture() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaAAC)
	binary.LittleEndian.PutUint16(b[4:6], 8)
	binary.LittleEndian.PutUint16(b[6:8], 8)
	copy(b[8:15], []byte{0xff, 0xf1, 0x50, 0x80, 0x01, 0x00, 0x00}) // ADTS frame length 8.
	b[15] = 0x55
	return b
}

func shortenedIFrame(successor []byte) []byte {
	b := make([]byte, 32+len(successor))
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 24)
	copy(b[24:32], []byte{0, 0, 0, 1, 1, 2, 3, 4})
	copy(b[32:], successor)
	return b
}

func TestMediaParserRejectsPlausibleVideoDecoyBeforeRealBoundary(t *testing.T) {
	const declared, decoy, real = 48, 36, 60
	realP := recoveryPFixture("H264")
	b := make([]byte, real+len(realP))
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], declared-mediaVideoHeaderSize)
	binary.LittleEndian.PutUint32(b[decoy:], mediaPFrameMin)
	copy(b[decoy+4:decoy+8], "H264")
	binary.LittleEndian.PutUint32(b[decoy+8:decoy+12], 5)
	copy(b[real:], realP)
	p, _ := NewMediaParser(Limits{})
	if packets, err := p.Append(b[:declared-2]); err != nil || len(packets) != 0 {
		t.Fatalf("unexpected pre-boundary fragment: packets=%d err=%v", len(packets), err)
	}
	if packets, err := p.Append(b[declared-2 : real+26]); err != nil || len(packets) != 0 {
		t.Fatalf("unexpected split-boundary fragment: packets=%d err=%v", len(packets), err)
	}
	packets, err := p.Append(b[real+26:])
	if err != nil || len(packets) != 2 || packets[0].Kind != MediaVideoI || packets[1].Kind != MediaVideoP {
		t.Fatalf("unexpected recovered boundary: packets=%+v err=%v", packets, err)
	}
	if len(packets[0].Data) != real-mediaVideoHeaderSize || packets[1].Codec != "H264" {
		t.Fatalf("selected wrong boundary: first=%d second=%+v", len(packets[0].Data), packets[1])
	}
}

func TestMediaParserExactBoundaryOvertakesPendingAdjustedCandidate(t *testing.T) {
	short := recoveryADPCMFixture()
	pframe := recoveryPFixture("H264")
	b := make([]byte, 48+len(pframe))
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 24)
	copy(b[24:32], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	copy(b[32:48], short)
	copy(b[48:], pframe)
	p, _ := NewMediaParser(Limits{})
	if packets, err := p.Append(b[:56]); err != nil || len(packets) != 0 {
		t.Fatalf("unexpected pending-candidate result: packets=%d err=%v", len(packets), err)
	}
	packets, err := p.Append(b[56:72])
	if err != nil || len(packets) != 1 || packets[0].Kind != MediaVideoI || len(packets[0].Data) != 24 {
		t.Fatalf("exact declared boundary did not win: packets=%+v err=%v", packets, err)
	}
	packets, err = p.Append(b[72:])
	if err != nil || len(packets) != 1 || packets[0].Kind != MediaVideoP {
		t.Fatalf("unexpected exact successor result: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserAcceptsShortenedIFrameSuccessors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		successor []byte
		kind      MediaKind
	}{
		{name: "H265 P-frame", successor: recoveryPFixture("H265"), kind: MediaVideoP},
		{name: "ADPCM", successor: recoveryADPCMFixture(), kind: MediaADPCM},
		{name: "AAC", successor: recoveryAACFixture(), kind: MediaAAC},
		{name: "info", successor: infoFixture(), kind: MediaInfo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := NewMediaParser(Limits{})
			fixture := shortenedIFrame(tt.successor)
			binary.LittleEndian.PutUint32(fixture[8:12], 32) // The corroborating info must not coincide with the declared end.
			packets, err := p.Append(append(fixture, infoFixture()...))
			if err != nil || len(packets) != 3 || packets[0].Kind != MediaVideoI || packets[1].Kind != tt.kind || packets[2].Kind != MediaInfo {
				t.Fatalf("unexpected shortened frame result: packets=%+v err=%v", packets, err)
			}
		})
	}
}

func TestMediaParserAcceptsConsecutiveAdjustedIFrames(t *testing.T) {
	second := shortenedIFrame(infoFixture())
	first := shortenedIFrame(second)
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(first)
	if err != nil || len(packets) != 3 || packets[0].Kind != MediaVideoI || packets[1].Kind != MediaVideoI || packets[2].Kind != MediaInfo {
		t.Fatalf("unexpected consecutive recovery result: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserRecoversVideoAtResyncLimit(t *testing.T) {
	const maxResync = 64
	for _, codec := range []string{"H264", "H265"} {
		for _, extra := range []int{0, 4, 8, 12} {
			t.Run(fmt.Sprintf("%s_extra_%d", codec, extra), func(t *testing.T) {
				p, err := NewMediaParser(Limits{MaxMediaFrame: 128, MaxMediaBuffer: 256, MaxResync: maxResync})
				if err != nil {
					t.Fatal(err)
				}
				offset := mediaVideoHeaderSize + maxResync
				fixture := keyframeBoundary(offset, []byte(codec))
				payloadStart := offset + mediaVideoHeaderSize
				payload := append([]byte(nil), fixture[payloadStart:]...)
				fixture = append(fixture[:payloadStart], make([]byte, extra)...)
				fixture = append(fixture, payload...)
				binary.LittleEndian.PutUint32(fixture[offset+12:offset+16], uint32(extra))

				packets, err := p.Append(fixture[:offset+8])
				if err != nil || len(packets) != 0 {
					t.Fatalf("rejected partial boundary: packets=%d err=%v", len(packets), err)
				}
				packets, err = p.Append(fixture[offset+8:])
				wantPayload := 5
				if codec == "H265" {
					wantPayload = 6
				}
				if err != nil || len(packets) != 2 || packets[0].Kind != MediaVideoI ||
					len(packets[0].Data) != maxResync || packets[1].Kind != MediaVideoP ||
					packets[1].Codec != codec || len(packets[1].Data) != wantPayload {
					t.Fatalf("incomplete recovery: packets=%+v err=%v", packets, err)
				}
			})
		}
	}
}

func TestMediaParserRecoversEarliestConsecutiveCutIFrames(t *testing.T) {
	good := recoveryPFixture("H264")
	binary.LittleEndian.PutUint32(good, mediaIFrameMin)
	second := shortenedIFrame(append(good, infoFixture()...))
	binary.LittleEndian.PutUint32(second[8:12], 100<<10)
	fixture := shortenedIFrame(second)
	binary.LittleEndian.PutUint32(fixture[8:12], 200<<10)
	p, _ := NewMediaParser(Limits{})
	if packets, err := p.Append(fixture); err != nil || len(packets) != 0 || p.scan != 0 {
		t.Fatalf("scanned before declared end: packets=%+v scan=%d err=%v", packets, p.scan, err)
	}
	packets, err := p.Append(make([]byte, mediaVideoHeaderSize+(200<<10)+4-len(fixture)))
	if err != nil || len(packets) != 4 || packets[0].Kind != MediaVideoI || !packets[0].ShortVideo ||
		packets[1].Kind != MediaVideoI || !packets[1].ShortVideo || packets[2].Kind != MediaVideoI ||
		packets[2].ShortVideo || packets[3].Kind != MediaInfo || len(packets[0].Data) != 8 || len(packets[1].Data) != 8 {
		t.Fatalf("lost earliest consecutive boundary: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserHealthyLargeIFrameDoesNotEnterRecovery(t *testing.T) {
	const payload = 2 << 20
	fixture := make([]byte, mediaVideoHeaderSize+payload)
	binary.LittleEndian.PutUint32(fixture, mediaIFrameMin)
	copy(fixture[4:8], "H264")
	binary.LittleEndian.PutUint32(fixture[8:12], payload)
	p, _ := NewMediaParser(Limits{})
	split := mediaRecoveryWindow + 128
	if packets, err := p.Append(fixture[:split]); err != nil || len(packets) != 0 || p.scan != 0 {
		t.Fatalf("large healthy frame entered recovery: packets=%d scan=%d err=%v", len(packets), p.scan, err)
	}
	packets, err := p.Append(append(fixture[split:], infoFixture()...))
	if err != nil || len(packets) != 2 || len(packets[0].Data) != payload || packets[0].ShortVideo {
		t.Fatalf("lost large healthy frame: packets=%d err=%v", len(packets), err)
	}
}

func TestMediaParserRecoversPFrameBeforeVideo(t *testing.T) {
	good := recoveryPFixture("H264")
	binary.LittleEndian.PutUint32(good, mediaIFrameMin)
	fixture := shortenedIFrame(append(good, append(infoFixture(), infoFixture()...)...))
	binary.LittleEndian.PutUint32(fixture, mediaPFrameMin)
	binary.LittleEndian.PutUint32(fixture[8:12], 81)
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(fixture)
	if err != nil || len(packets) != 4 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaVideoI ||
		packets[2].Kind != MediaInfo || packets[3].Kind != MediaInfo {
		t.Fatalf("failed P-to-video recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserMissingPaddingNeedsCorroboratedAudio(t *testing.T) {
	for _, tt := range []struct {
		name  string
		audio []byte
		kind  MediaKind
	}{
		{name: "AAC", audio: recoveryAACFixture(), kind: MediaAAC},
		{name: "ADPCM", audio: recoveryADPCMFixture(), kind: MediaADPCM},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := NewMediaParser(Limits{})
			if packets, err := p.Append(append(videoFixture()[:35], tt.audio[:10]...)); err != nil || len(packets) != 0 {
				t.Fatalf("accepted incomplete audio anchor: packets=%+v err=%v", packets, err)
			}
			info := infoFixture()
			if packets, err := p.Append(append(tt.audio[10:], info[:6]...)); err != nil || len(packets) != 0 {
				t.Fatalf("accepted incomplete audio successor: packets=%+v err=%v", packets, err)
			}
			packets, err := p.Append(info[6:])
			if err != nil || len(packets) != 3 || packets[0].Kind != MediaVideoI || packets[0].ShortVideo ||
				len(packets[0].Data) != 3 || packets[1].Kind != tt.kind || packets[2].Kind != MediaInfo {
				t.Fatalf("lost intact padding-only keyframe: packets=%+v err=%v", packets, err)
			}
		})
	}
}

func TestMediaParserPendingPlainEndpointPrecedesRecovery(t *testing.T) {
	const plain, declared = 131, 136
	wire := bytes.Repeat([]byte{0x55}, plain+32)
	binary.LittleEndian.PutUint32(wire, mediaIFrameMin)
	copy(wire[4:8], "H264")
	binary.LittleEndian.PutUint32(wire[8:12], plain-mediaVideoHeaderSize)
	binary.LittleEndian.PutUint32(wire[12:16], 0)
	earlier := recoveryPFixture("H264")
	binary.LittleEndian.PutUint32(earlier, mediaIFrameMin)
	copy(wire[32:], earlier)
	copy(wire[plain:], infoFixture())
	for _, split := range []int{140, len(wire)} {
		p, _ := NewMediaParser(Limits{})
		packets, err := p.Append(wire[:split])
		if err != nil || split < len(wire) && (len(packets) != 0 || p.scan != 0) {
			t.Fatalf("plain endpoint lost priority at split %d: packets=%+v scan=%d err=%v", split, packets, p.scan, err)
		}
		next, err := p.Append(wire[split:])
		packets = append(packets, next...)
		if err != nil || len(packets) != 2 || packets[0].Kind != MediaVideoI || packets[0].ShortVideo ||
			!bytes.Equal(packets[0].Data, wire[mediaVideoHeaderSize:plain]) || packets[1].Kind != MediaInfo {
			t.Fatalf("split %d selected recovery over declared %d/%d: packets=%+v err=%v", split, plain, declared, packets, err)
		}
	}
}

func TestMediaParserRecoversNearLargeDeclaredIFrame(t *testing.T) {
	const declared = 2 << 20
	for _, delta := range []int{-64 << 10, 64 << 10} {
		offset := declared + delta
		wire := bytes.Repeat([]byte{0x55}, offset)
		binary.LittleEndian.PutUint32(wire, mediaIFrameMin)
		copy(wire[4:8], "H264")
		binary.LittleEndian.PutUint32(wire[8:12], declared-mediaVideoHeaderSize)
		binary.LittleEndian.PutUint32(wire[12:16], 0)
		wire = append(wire, recoveryPFixture("H264")...)
		tail := bytes.Repeat([]byte{0x55}, mediaVideoHeaderSize+(128<<10))
		binary.LittleEndian.PutUint32(tail, mediaPFrameMin)
		copy(tail[4:8], "H264")
		binary.LittleEndian.PutUint32(tail[8:12], 128<<10)
		binary.LittleEndian.PutUint32(tail[12:16], 0)
		copy(tail[mediaVideoHeaderSize:], []byte{0, 0, 0, 1, 1})
		wire = append(append(wire, tail...), infoFixture()...)
		for _, split := range []int{declared + 4, len(wire)} {
			p, _ := NewMediaParser(Limits{})
			packets, err := p.Append(wire[:split])
			if err != nil {
				t.Fatalf("delta %d split %d: %v", delta, split, err)
			}
			next, err := p.Append(wire[split:])
			packets = append(packets, next...)
			if err != nil || len(packets) != 4 || packets[0].Kind != MediaVideoI || packets[0].ShortVideo != (delta < 0) ||
				len(packets[0].Data) != offset-mediaVideoHeaderSize || packets[1].Kind != MediaVideoP ||
				packets[2].Kind != MediaVideoP || packets[3].Kind != MediaInfo || len(p.buf) != 0 {
				t.Fatalf("lost large-I boundary at delta %d split %d: packets=%+v err=%v", delta, split, packets, err)
			}
		}
	}
}

func TestMediaParserPlainAudioMayExtendBeyondFrameLimit(t *testing.T) {
	const plain, declared, payload = 1019, 1024, 256
	wire := bytes.Repeat([]byte{0x55}, plain)
	binary.LittleEndian.PutUint32(wire, mediaIFrameMin)
	copy(wire[4:8], "H264")
	binary.LittleEndian.PutUint32(wire[8:12], plain-mediaVideoHeaderSize)
	binary.LittleEndian.PutUint32(wire[12:16], 0)
	audio := bytes.Repeat([]byte{0x55}, 8+payload)
	binary.LittleEndian.PutUint32(audio, mediaADPCM)
	binary.LittleEndian.PutUint16(audio[4:6], payload)
	binary.LittleEndian.PutUint16(audio[6:8], payload)
	binary.LittleEndian.PutUint16(audio[8:10], 0x0100)
	wire = append(append(wire, audio...), infoFixture()...)
	p, _ := NewMediaParser(Limits{MaxMediaFrame: declared, MaxMediaBuffer: 128 << 10})
	split := plain + 128
	if packets, err := p.Append(wire[:split]); err != nil || len(packets) != 0 || p.scan != 0 {
		t.Fatalf("partial audio lost declared priority: packets=%+v scan=%d err=%v", packets, p.scan, err)
	}
	packets, err := p.Append(wire[split:])
	if err != nil || len(packets) != 3 || packets[0].Kind != MediaVideoI || packets[0].ShortVideo ||
		len(packets[0].Data) != plain-mediaVideoHeaderSize || packets[1].Kind != MediaADPCM || packets[2].Kind != MediaInfo {
		t.Fatalf("frame limit incorrectly capped successor confirmation: packets=%+v err=%v", packets, err)
	}
}
