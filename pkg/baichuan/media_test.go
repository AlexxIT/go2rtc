package baichuan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func videoFixture() []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H265")
	binary.LittleEndian.PutUint32(b[8:12], 3)
	binary.LittleEndian.PutUint32(b[12:16], 8)
	binary.LittleEndian.PutUint32(b[16:20], 123456)
	binary.LittleEndian.PutUint32(b[24:28], 1000)
	copy(b[32:35], []byte{1, 2, 3})
	return b
}

func TestMediaParserIFrameExtraHeader(t *testing.T) {
	for _, extra := range []uint32{0, 4, 8, 12} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			const payload = 3
			start := 24 + int(extra)
			video := make([]byte, start+8)
			binary.LittleEndian.PutUint32(video, mediaIFrameMin)
			copy(video[4:8], "H265")
			binary.LittleEndian.PutUint32(video[8:12], payload)
			binary.LittleEndian.PutUint32(video[12:16], extra)
			binary.LittleEndian.PutUint32(video[16:20], 123456)
			if extra >= 4 {
				binary.LittleEndian.PutUint32(video[24:28], 1000)
			}
			copy(video[start:start+payload], []byte{1, 2, 3})

			p, _ := NewMediaParser(Limits{})
			packets, err := p.Append(append(video, infoFixture()...))
			if err != nil || len(packets) != 2 {
				t.Fatalf("unexpected result: packets=%d err=%v", len(packets), err)
			}
			packet := packets[0]
			if !bytes.Equal(packet.Data, []byte{1, 2, 3}) || packet.Timestamp != 123456 {
				t.Fatalf("unexpected packet: %+v", packet)
			}
			if extra >= 4 && packet.WallTime.Unix() != 1000 || extra < 4 && !packet.WallTime.IsZero() {
				t.Fatalf("unexpected wall time: %v", packet.WallTime)
			}
		})
	}
}

func infoFixture() []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, mediaInfoV2)
	binary.LittleEndian.PutUint32(b[4:8], 32)
	binary.LittleEndian.PutUint32(b[8:12], 3840)
	binary.LittleEndian.PutUint32(b[12:16], 2160)
	b[17] = 20
	return b
}

func TestMediaParserSplitAndOwnership(t *testing.T) {
	p, err := NewMediaParser(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	fixture := videoFixture()
	packets, err := p.Append(fixture[:19])
	if err != nil || len(packets) != 0 {
		t.Fatalf("unexpected partial result: %d, %v", len(packets), err)
	}
	packets, err = p.Append(append(fixture[19:], infoFixture()...))
	if err != nil || len(packets) != 2 {
		t.Fatalf("unexpected complete result: %d, %v", len(packets), err)
	}
	packet := packets[0]
	if packet.Kind != MediaVideoI || packet.Codec != "H265" || packet.Timestamp != 123456 ||
		!packet.WallTime.Equal(packet.WallTime.UTC()) || !bytes.Equal(packet.Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected packet: %+v", packet)
	}
	fixture[32] = 9
	if packet.Data[0] != 1 {
		t.Fatal("packet data aliases parser input")
	}
}

func TestMediaParserRecoversUnknownPrefix(t *testing.T) {
	for length := 1; length <= 4; length++ {
		p, _ := NewMediaParser(Limits{})
		packets, err := p.Append(append([]byte{9, 8, 7, 6}[:length], infoFixture()...))
		if err != nil || len(packets) != 2 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaInfo {
			t.Fatalf("unexpected unknown-prefix recovery at %d: packets=%+v err=%v", length, packets, err)
		}
	}
}

func TestMediaParserDataAppendPreservesResidue(t *testing.T) {
	pframe := videoFixture()
	binary.LittleEndian.PutUint32(pframe, mediaPFrameMin)
	aac := make([]byte, 16)
	binary.LittleEndian.PutUint32(aac, mediaAAC)
	binary.LittleEndian.PutUint16(aac[4:6], 8)
	binary.LittleEndian.PutUint16(aac[6:8], 8)
	adpcm := bytes.Clone(aac)
	binary.LittleEndian.PutUint32(adpcm, mediaADPCM)
	binary.LittleEndian.PutUint16(adpcm[8:10], 0x0100)
	for _, wire := range [][]byte{videoFixture(), pframe, aac, adpcm} {
		t.Run(fmt.Sprintf("%x", wire[:4]), func(t *testing.T) {
			p, _ := NewMediaParser(Limits{})
			info := infoFixture()
			packets, err := p.Append(append(wire, info[:8]...))
			if err != nil || len(packets) != 1 {
				t.Fatalf("unexpected packet: %+v err=%v", packets, err)
			}
			data := packets[0].Data
			if cap(data) != len(data) {
				t.Errorf("packet capacity %d exceeds length %d", cap(data), len(data))
			}
			_ = append(data, bytes.Repeat([]byte{0xff}, 8)...)
			packets, err = p.Append(info[8:])
			if err != nil || len(packets) != 1 || packets[0].Kind != MediaInfo || packets[0].Width != 3840 {
				t.Fatalf("appending to packet corrupted residue: %+v err=%v", packets, err)
			}
		})
	}
}

func TestMediaParserAdjustsBrokenKeyframeLength(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	video := videoFixture()[:35]
	binary.LittleEndian.PutUint32(video[8:12], 100)
	fixture := append(video, infoFixture()...)
	if packets, err := p.Append(fixture); err != nil || len(packets) != 0 {
		t.Fatalf("scanned before declared end: packets=%+v err=%v", packets, err)
	}
	packets, err := p.Append(make([]byte, 140-len(fixture)))
	if err != nil || len(packets) != 2 {
		t.Fatalf("unexpected result: packets=%d err=%v", len(packets), err)
	}
	if !bytes.Equal(packets[0].Data, []byte{1, 2, 3}) || !packets[0].ShortVideo {
		t.Fatalf("unexpected keyframe data: %x", packets[0].Data)
	}
	if got := p.Stats(); got.BoundaryAdjustments != 1 || got.ShortVideoUnits != 1 {
		t.Fatalf("media stats = %+v, want one adjustment and one short unit", got)
	}
	if got := (&Preview{parser: p}).Stats(); got.BoundaryAdjustments != 1 || got.ShortVideoUnits != 1 {
		t.Fatalf("preview media stats = %+v, want one adjustment and one short unit", got)
	}
}

func TestMediaParserPFrameExtraHeader(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 36)
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], "H265")
	binary.LittleEndian.PutUint32(b[8:12], 3)
	binary.LittleEndian.PutUint32(b[12:16], 4)
	copy(b[28:31], []byte{1, 2, 3})
	packets, err := p.Append(b)
	if err != nil || len(packets) != 1 || !bytes.Equal(packets[0].Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected P-frame result: %+v err=%v", packets, err)
	}
}

func TestMediaParserRecoversADPCMUnderflow(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaADPCM)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 2)
	packets, err := p.Append(append(b, infoFixture()...))
	if err != nil || len(packets) != 2 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaInfo {
		t.Fatalf("unexpected ADPCM recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserRecoversInconsistentAudioSize(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaAAC)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 3)
	packets, err := p.Append(append(b, infoFixture()...))
	if err != nil || len(packets) != 2 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaInfo {
		t.Fatalf("unexpected audio recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserUnpaddedFragmentDoesNotConsumeNextHeader(t *testing.T) {
	audio := make([]byte, 13)
	binary.LittleEndian.PutUint32(audio, mediaAAC)
	binary.LittleEndian.PutUint16(audio[4:6], 5)
	binary.LittleEndian.PutUint16(audio[6:8], 5)
	copy(audio[8:], "audio")
	info := infoFixture()
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(append(audio, info[:3]...))
	if err != nil || len(packets) != 0 {
		t.Fatalf("unexpected first fragment: packets=%d err=%v", len(packets), err)
	}
	packets, err = p.Append(info[3:])
	if err != nil || len(packets) != 2 || string(packets[0].Data) != "audio" || packets[1].Kind != MediaInfo {
		t.Fatalf("unexpected completed packets: %+v err=%v", packets, err)
	}
}

func TestMediaParserFrameLimit(t *testing.T) {
	p, err := NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 100)
	if packets, err := p.Append(b); err != nil || len(packets) != 0 {
		t.Fatalf("rejected header did not wait for recovery: packets=%+v err=%v", packets, err)
	}
	if _, err = p.Append(make([]byte, 64+mediaRecoveryLookahead)); err == nil || !strings.Contains(err.Error(), "video frame 128 exceeds limit 64") {
		t.Fatalf("unexpected error: %v", err)
	}

	p, _ = NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	binary.LittleEndian.PutUint32(b[8:12], 0)
	binary.LittleEndian.PutUint32(b[12:16], ^uint32(0))
	if packets, err := p.Append(b); err != nil || len(packets) != 0 {
		t.Fatalf("rejected I-header did not wait for recovery: packets=%+v err=%v", packets, err)
	}
	if _, err = p.Append(make([]byte, 64+mediaRecoveryLookahead)); err == nil || !strings.Contains(err.Error(), "video frame 4294967319 exceeds limit 64") {
		t.Fatalf("unexpected I-frame error: %v", err)
	}

	p, _ = NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	b = make([]byte, 72)
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 40)
	if _, err = p.Append(append(b, infoFixture()...)); !errors.Is(err, errNoKeyframeBoundary) {
		t.Fatalf("accepted adjusted I-frame beyond limit: %v", err)
	}
}

func TestMediaParserAppendLimit(t *testing.T) {
	p, err := NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Append(make([]byte, 161)); err == nil || !strings.Contains(err.Error(), "media buffer exceeds limit 160") {
		t.Fatalf("unexpected append error: %v", err)
	}
	if p.buf != nil {
		t.Fatal("oversized append changed parser state")
	}
}

func TestMediaParserPacketLimit(t *testing.T) {
	frame := recoveryAACFixture()
	wire := bytes.Repeat(frame, 4097)
	for _, tt := range []struct {
		name    string
		ends    []int
		counts  []int
		wantErr bool
	}{
		{"at limit", []int{4096 * len(frame)}, []int{4096}, false},
		{"over limit", []int{len(wire)}, []int{4096}, true},
		{"fragmented across appends", []int{4096*len(frame) + 7, len(wire)}, []int{4096, 1}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewMediaParser(Limits{})
			if err != nil {
				t.Fatal(err)
			}
			var packets []MediaPacket
			start := 0
			for i, end := range tt.ends {
				packets, err = p.appendOwnedTo(wire[start:end], packets)
				if len(packets) != tt.counts[i] || (err != nil) != tt.wantErr {
					t.Fatalf("append %d: packets=%d err=%v", i, len(packets), err)
				}
				if err != nil && (!strings.Contains(err.Error(), "media packet count exceeds limit 4096") || len(p.buf) != len(frame)) {
					t.Fatalf("unexpected admission failure: remaining=%d err=%v", len(p.buf), err)
				}
				for _, packet := range packets {
					if packet.Kind != MediaAAC || !bytes.Equal(packet.Data, frame[8:]) {
						t.Fatal("nonempty AAC packet changed")
					}
				}
				start = end
			}
			if !tt.wantErr && len(p.buf) != 0 {
				t.Fatalf("retained %d bytes", len(p.buf))
			}
		})
	}
	t.Run("adjusted I-frame counters commit after packet limit", func(t *testing.T) {
		p, err := NewMediaParser(Limits{})
		if err != nil {
			t.Fatal(err)
		}
		wire := bytes.Repeat(frame, maxMediaPackets)
		wire = append(wire, shortenedIFrame(infoFixture())...)
		wire = append(wire, infoFixture()...)

		packets, err := p.appendOwnedTo(wire, nil)
		if err == nil || !strings.Contains(err.Error(), "media packet count exceeds limit 4096") || len(packets) != maxMediaPackets {
			t.Fatalf("packet-limit result: packets=%d err=%v", len(packets), err)
		}
		if len(p.buf) != len(shortenedIFrame(infoFixture()))+len(infoFixture()) {
			t.Fatalf("adjusted frame was consumed at the limit: retained=%d", len(p.buf))
		}
		if got := p.Stats(); got != (MediaStats{}) {
			t.Fatalf("stats counted an unconsumed adjusted frame: %+v", got)
		}

		packets, err = p.appendOwnedTo(nil, packets)
		if err != nil || len(packets) != 3 || packets[0].Kind != MediaVideoI || !packets[0].ShortVideo ||
			packets[1].Kind != MediaInfo || packets[2].Kind != MediaInfo || len(p.buf) != 0 {
			t.Fatalf("deferred adjusted frame result: packets=%+v retained=%d err=%v", packets, len(p.buf), err)
		}
		wantStats := MediaStats{BoundaryAdjustments: 1, ShortVideoUnits: 1}
		if got := p.Stats(); got != wantStats {
			t.Fatalf("stats after consuming adjusted frame = %+v, want %+v", got, wantStats)
		}
		packets, err = p.appendOwnedTo(nil, packets)
		if err != nil || len(packets) != 0 {
			t.Fatalf("empty retry: packets=%d err=%v", len(packets), err)
		}
		if got := p.Stats(); got != wantStats {
			t.Fatalf("empty retry changed counters = %+v, want %+v", got, wantStats)
		}
	})
	t.Run("preview rejects entire batch", func(t *testing.T) {
		parser, _ := NewMediaParser(Limits{})
		preview := &Preview{
			client:   &Client{previews: make(map[previewKey]*Preview)},
			messages: make(chan message, 1), queueLimit: int64(len(wire)),
			done: make(chan struct{}), parser: parser,
		}
		preview.deliver(message{payload: wire})
		packet, err := preview.Read(context.Background())
		if err == nil || !strings.Contains(err.Error(), "media packet count exceeds limit 4096") || packet.Kind != 0 || len(preview.packets) != 0 {
			t.Fatalf("oversized batch escaped: kind=%d packets=%d err=%v", packet.Kind, len(preview.packets), err)
		}
	})
}

func brokenPWithAAC() []byte {
	b := make([]byte, 48)
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 81)
	audio := make([]byte, 24)
	binary.LittleEndian.PutUint32(audio, mediaAAC)
	binary.LittleEndian.PutUint16(audio[4:6], 13)
	binary.LittleEndian.PutUint16(audio[6:8], 13)
	copy(audio[8:], []byte{0xff, 0xf1, 0x50, 0x80, 0x01, 0xa0, 0})
	b = append(b, audio...)
	b = append(b, infoFixture()...)
	return append(b, infoFixture()...)
}

func TestMediaParserRecoversCorroboratedPBoundary(t *testing.T) {
	fixture := brokenPWithAAC()
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(append(infoFixture(), fixture...))
	if err != nil || len(packets) != 5 || packets[0].Kind != MediaInfo ||
		packets[1].Kind != MediaDiscontinuity || packets[2].Kind != MediaAAC ||
		packets[3].Kind != MediaInfo || packets[4].Kind != MediaInfo {
		t.Fatalf("incorrect recovery order: packets=%+v err=%v", packets, err)
	}

	p, _ = NewMediaParser(Limits{})
	if packets, err = p.Append(fixture[:100]); err != nil || len(packets) != 0 {
		t.Fatalf("unexpected partial result: packets=%d err=%v", len(packets), err)
	}
	if packets, err = p.Append(fixture[100:]); err != nil || len(packets) != 4 || packets[0].Kind != MediaDiscontinuity {
		t.Fatalf("fragmented recovery failed: packets=%+v err=%v", packets, err)
	}

	for _, damage := range []func([]byte){
		func(b []byte) { b[48+8+4] = 2 }, // AAC size differs from ADTS.
		func(b []byte) { b[72] = 0 },     // The following header is invalid.
		func(b []byte) { // The following AAC header lacks a matching ADTS header.
			binary.LittleEndian.PutUint32(b[72:], mediaAAC)
			binary.LittleEndian.PutUint16(b[76:78], 13)
			binary.LittleEndian.PutUint16(b[78:80], 13)
		},
		func(b []byte) { b[69] = 1 }, // AAC padding is not zero.
		func(b []byte) { // AAC lies inside extra header, with a complete mismatched frame.
			binary.LittleEndian.PutUint32(b[12:16], 30)
			binary.LittleEndian.PutUint32(b[8:12], 53)
		},
	} {
		b := brokenPWithAAC()
		damage(b)
		p, _ = NewMediaParser(Limits{})
		packets, err := p.Append(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, packet := range packets {
			if packet.Kind == MediaAAC {
				t.Fatalf("accepted unsafe AAC anchor: packets=%+v", packets)
			}
		}
	}
	p, _ = NewMediaParser(Limits{MaxMediaFrame: 256, MaxMediaBuffer: 352, MaxResync: 64})
	if _, err = p.Append(fixture); err == nil {
		t.Fatal("recovered P frame larger than full scan window")
	}
	p, _ = NewMediaParser(Limits{})
	packets, err = p.Append(append(fixture, fixture...))
	if err != nil || len(packets) != 8 || packets[0].Kind != MediaDiscontinuity || packets[4].Kind != MediaDiscontinuity {
		t.Fatalf("failed repeated recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserPaddingOnlyBoundaryDoesNotMarkVideoShort(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	video := videoFixture()[:35] // payload ends here; only the declared alignment padding is absent
	packets, err := p.Append(append(video, infoFixture()...))
	if err != nil || len(packets) != 2 || packets[0].ShortVideo {
		t.Fatalf("padding-only boundary marked video short: packets=%+v err=%v", packets, err)
	}
	if got := p.Stats(); got.BoundaryAdjustments != 1 || got.ShortVideoUnits != 0 {
		t.Fatalf("media stats = %+v, want one adjustment and no short units", got)
	}
	if got := (&Preview{parser: p}).Stats(); got.BoundaryAdjustments != 1 || got.ShortVideoUnits != 0 {
		t.Fatalf("preview media stats = %+v, want one adjustment and no short units", got)
	}
}
