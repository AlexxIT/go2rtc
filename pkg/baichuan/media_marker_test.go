package baichuan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func markerIFrame(codec string, payload []byte, declared uint32, timestamp uint32) []byte {
	b := make([]byte, mediaVideoHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], codec)
	binary.LittleEndian.PutUint32(b[8:12], declared)
	binary.LittleEndian.PutUint32(b[16:20], timestamp)
	copy(b[mediaVideoHeaderSize:], payload)
	return b
}

func markerPreview(t *testing.T) *Preview {
	t.Helper()
	parser, err := NewMediaParser(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{done: make(chan struct{}), previews: make(map[previewKey]*Preview)}
	return &Preview{client: client, messages: make(chan message, 16), queueLimit: int64(parser.limits.MaxMediaBuffer),
		done: make(chan struct{}), parser: parser}
}

func enqueueMarker(p *Preview, data []byte, encryptLen *int) {
	msg := message{binary: true, payload: data}
	if encryptLen != nil {
		msg.hasEncryptLen = true
		msg.encrypt = *encryptLen
	}
	p.deliver(msg)
}

func enqueuePooledMarker(t *testing.T, p *Preview, data []byte) {
	t.Helper()
	body := p.client.readBuffers.get(previewBufferMin)
	if len(data) > len(body)-16 {
		t.Fatal("pooled marker payload exceeds body")
	}
	copy(body[16:], data)
	p.deliver(message{binary: true, extension: body[:16], payload: body[16 : 16+len(data)], recycleBody: body})
}

func readMarkerPacket(t *testing.T, p *Preview) (MediaPacket, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return p.Read(ctx)
}

func positiveMarker() *int {
	n := 1
	return &n
}

func enqueueMarkerQualification(t *testing.T, p *Preview) {
	t.Helper()
	i := markerIFrame("H264", []byte{0, 0, 0, 1, 1, 2, 3, 4}, 8, 100)
	enqueueMarker(p, i[:mediaVideoHeaderSize], positiveMarker())
	enqueueMarker(p, i[mediaVideoHeaderSize:], nil)
	enqueueMarker(p, recoveryPFixture("H264"), positiveMarker())
}

func appendMarkerTestMessage(t *testing.T, p *MediaParser, payload []byte, marked bool) []MediaPacket {
	t.Helper()
	msg := message{binary: true, payload: payload}
	if marked {
		msg.hasEncryptLen, msg.encrypt = true, 1
	}
	packets, err := p.appendMessage(msg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return packets
}

func qualifiedMarkerParser(t *testing.T) *MediaParser {
	t.Helper()
	p, err := NewMediaParser(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	i := markerIFrame("H264", []byte{0, 0, 0, 1, 1, 2, 3, 4}, 8, 100)
	appendMarkerTestMessage(t, p, i[:mediaVideoHeaderSize], true)
	appendMarkerTestMessage(t, p, i[mediaVideoHeaderSize:], false)
	packets := appendMarkerTestMessage(t, p, recoveryPFixture("H264"), true)
	if len(packets) != 2 || packets[0].Kind != MediaVideoI || packets[1].Kind != MediaVideoP || !p.markerQualified {
		t.Fatalf("marker fixture did not qualify: packets=%+v qualified=%t", packets, p.markerQualified)
	}
	return p
}

func TestPreviewQualifiedMarkerRecoversShortIAtValidatedVideoStart(t *testing.T) {
	p := markerPreview(t)
	enqueueMarkerQualification(t, p)
	for _, want := range []MediaKind{MediaVideoI, MediaVideoP} {
		packet, err := readMarkerPacket(t, p)
		if err != nil || packet.Kind != want {
			t.Fatalf("qualification packet = %+v, %v; want %v", packet, err, want)
		}
	}

	payload := []byte{0, 0, 0, 1, 1, 2, 3, 4}
	short := markerIFrame("H264", payload, 4096, 200)
	enqueueMarker(p, short[:mediaVideoHeaderSize+4], positiveMarker())
	enqueuePooledMarker(t, p, short[mediaVideoHeaderSize+4:])
	enqueueMarker(p, recoveryPFixture("H264"), positiveMarker())
	first, err := readMarkerPacket(t, p)
	if err != nil || first.Kind != MediaVideoI || !first.ShortVideo || first.Timestamp != 200 || !bytes.Equal(first.Data, payload) {
		t.Fatalf("qualified short I = %+v, %v", first, err)
	}
	if cached := p.client.readBuffers.cached(); cached != 1 {
		t.Fatalf("copied preview body returned %d pooled buffers, want 1", cached)
	}
	reused := p.client.readBuffers.get(previewBufferMin)
	for i := range reused {
		reused[i] = 0xa5
	}
	if !bytes.Equal(first.Data, payload) {
		t.Fatal("reusing pooled body changed returned video data")
	}
	second, err := readMarkerPacket(t, p)
	if err != nil || second.Kind != MediaVideoP {
		t.Fatalf("marked successor = %+v, %v", second, err)
	}
	if got := p.Stats(); got.BoundaryAdjustments != 1 || got.ShortVideoUnits != 1 {
		t.Fatalf("short-I accounting = %+v, want one adjustment", got)
	}
}

func TestPreviewMarkerHintRequiresSessionQualification(t *testing.T) {
	for _, marked := range []bool{true, false} {
		t.Run(map[bool]string{true: "first truncation", false: "binary flag only"}[marked], func(t *testing.T) {
			p := markerPreview(t)
			short := markerIFrame("H264", []byte{0, 0, 0, 1, 1, 2, 3, 4}, 4096, 300)
			var marker *int
			if marked {
				marker = positiveMarker()
			}
			enqueueMarker(p, short, marker)
			enqueueMarker(p, recoveryPFixture("H264"), positiveMarker())
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			packet, err := p.Read(ctx)
			if err == nil || !errors.Is(err, context.DeadlineExceeded) || packet.Kind != 0 {
				t.Fatalf("unqualified marker produced packet %+v, err %v", packet, err)
			}
			if got := p.Stats(); got.BoundaryAdjustments != 0 || got.ShortVideoUnits != 0 {
				t.Fatalf("unexpected adjustment accounting: %+v", got)
			}
		})
	}
}

func TestMediaParserPublicAppendHasNoMarkerAuthority(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	short := markerIFrame("H264", []byte{0, 0, 0, 1, 1, 2, 3, 4}, 4096, 400)
	if packets, err := p.Append(short); err != nil || len(packets) != 0 {
		t.Fatalf("first short frame: packets=%+v err=%v", packets, err)
	}
	packets, err := p.Append(recoveryPFixture("H264"))
	if err != nil || len(packets) != 0 || p.Stats().ShortVideoUnits != 0 {
		t.Fatalf("raw Append acquired marker authority: packets=%+v stats=%+v err=%v", packets, p.Stats(), err)
	}
}

func TestMarkerContradictorySpanClearsQualification(t *testing.T) {
	p := qualifiedMarkerParser(t)
	if got := appendMarkerTestMessage(t, p, infoFixture()[:4], false); len(got) != 0 || !p.markerQualified {
		t.Fatalf("partial unmarked prefix changed qualification: packets=%+v qualified=%t", got, p.markerQualified)
	}
	appendMarkerTestMessage(t, p, infoFixture(), true) // starts four bytes past the prior declared endpoint
	if p.markerQualified {
		t.Fatal("contradictory marked span retained qualification")
	}
}

func TestMarkerHintDoesNotShortenPFrame(t *testing.T) {
	p := qualifiedMarkerParser(t)
	frame := recoveryPFixture("H264")[:mediaVideoHeaderSize]
	binary.LittleEndian.PutUint32(frame[8:12], 4096)
	if got := appendMarkerTestMessage(t, p, frame, true); len(got) != 0 || !p.markerQualified {
		t.Fatalf("partial P-frame start changed qualification: packets=%+v qualified=%t", got, p.markerQualified)
	}
	appendMarkerTestMessage(t, p, []byte{0, 0, 0, 1, 1}, false)
	got := appendMarkerTestMessage(t, p, recoveryPFixture("H264"), true)
	if len(got) != 0 || p.markerQualified || p.Stats() != (MediaStats{}) {
		t.Fatalf("P-frame residue was shortened: packets=%+v qualified=%t stats=%+v", got, p.markerQualified, p.Stats())
	}
}

func TestMarkerPartialPaddingUsesRawFallback(t *testing.T) {
	p := qualifiedMarkerParser(t)
	frame := markerIFrame("H264", bytes.Repeat([]byte{0x55}, 4099), 4097, 500)
	successor := recoveryPFixture("H264")
	var got []MediaPacket
	got = append(got, appendMarkerTestMessage(t, p, frame[:mediaVideoHeaderSize], true)...)
	got = append(got, appendMarkerTestMessage(t, p, frame[mediaVideoHeaderSize:], false)...)
	got = append(got, appendMarkerTestMessage(t, p, successor, true)...)
	raw, _ := NewMediaParser(Limits{})
	want, err := raw.Append(append(append([]byte(nil), frame...), successor...))
	if err != nil || len(got) != 2 || len(got) != len(want) || p.markerQualified || p.Stats() != raw.Stats() {
		t.Fatalf("partial-padding marker path: packets=%+v qualified=%t stats=%+v want=%+v stats=%+v err=%v", got, p.markerQualified, p.Stats(), want, raw.Stats(), err)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].ShortVideo != want[i].ShortVideo || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Fatalf("fallback packet %d differs: got=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestMarkerExactDeclaredBoundaryWinsOverHint(t *testing.T) {
	p := qualifiedMarkerParser(t)
	i := markerIFrame("H264", bytes.Repeat([]byte{0x55}, 40), 40, 600)
	copy(i[mediaVideoHeaderSize:mediaVideoHeaderSize+8], []byte{0, 0, 0, 1, 1, 2, 3, 4})
	pframe := recoveryPFixture("H264")
	copy(i[32:64], pframe)
	if !confirmedVideoMarker(i[32:], p.limits.MaxMediaFrame) {
		t.Fatal("fixture lacks a confirmed in-payload marker candidate")
	}
	appendMarkerTestMessage(t, p, i[:32], true)
	got := appendMarkerTestMessage(t, p, append(append([]byte(nil), i[32:]...), pframe...), true)
	if len(got) != 2 || got[0].Kind != MediaVideoI || got[0].ShortVideo || len(got[0].Data) != 40 ||
		got[1].Kind != MediaVideoP {
		t.Fatalf("declared endpoint did not win: packets=%+v", got)
	}
}

func markerFuzzByte(data []byte, cursor *int, fallback byte) byte {
	if len(data) == 0 {
		return fallback
	}
	value := data[*cursor%len(data)]
	(*cursor)++
	return value
}

func appendMarkerFuzzChunks(t *testing.T, p *MediaParser, payload, data []byte, cursor *int) []MediaPacket {
	var packets []MediaPacket
	for len(payload) > 0 {
		size := int(markerFuzzByte(data, cursor, 0)%8) + 1
		if size > len(payload) {
			size = len(payload)
		}
		packets = append(packets, appendMarkerTestMessage(t, p, payload[:size], false)...)
		payload = payload[size:]
	}
	return packets
}

func FuzzPreviewMarkerMessages(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{1, 7, 3, 11, 5, 13})
	f.Fuzz(func(t *testing.T, data []byte) {
		data = data[:min(len(data), 64)]
		p, err := NewMediaParser(Limits{MaxMediaFrame: 8192, MaxMediaBuffer: 32768, MaxResync: 8192})
		if err != nil {
			t.Fatal(err)
		}
		var cursor int
		cleanData := []byte{0, 0, 0, 1, 1, markerFuzzByte(data, &cursor, 2), markerFuzzByte(data, &cursor, 3), markerFuzzByte(data, &cursor, 4)}
		clean := markerIFrame("H264", cleanData, uint32(len(cleanData)), 100)
		appendMarkerTestMessage(t, p, clean[:mediaVideoHeaderSize], true)
		appendMarkerFuzzChunks(t, p, clean[mediaVideoHeaderSize:], data, &cursor)
		qualified := appendMarkerTestMessage(t, p, recoveryPFixture("H264"), true)
		if !p.markerQualified || len(qualified) != 2 || qualified[0].Kind != MediaVideoI || qualified[1].Kind != MediaVideoP {
			t.Fatalf("seed did not hit marker qualification: packets=%+v qualified=%t", qualified, p.markerQualified)
		}

		audio := recoveryAACFixture()
		audio[len(audio)-1] = markerFuzzByte(data, &cursor, 0x55)
		short := markerIFrame("H264", audio, 4096, uint32(markerFuzzByte(data, &cursor, 0)))
		appendMarkerTestMessage(t, p, short[:mediaVideoHeaderSize], true)
		appendMarkerFuzzChunks(t, p, short[mediaVideoHeaderSize:], data, &cursor)
		packets := appendMarkerTestMessage(t, p, recoveryPFixture("H264"), true)
		for _, packet := range packets {
			if packet.Kind != MediaVideoI || !packet.ShortVideo {
				continue
			}
			count := 0
			err := packet.RecoverAudio(func(got MediaPacket) error {
				count++
				if got.Kind != MediaAAC || !bytes.Equal(got.Data, audio[8:]) {
					t.Fatalf("short-video audio recovery = %+v", got)
				}
				return nil
			})
			if err != nil || count != 1 {
				t.Fatalf("ShortVideo.RecoverAudio endpoint count=%d err=%v", count, err)
			}
			return
		}
		t.Fatalf("seed did not hit marker hint and ShortVideo endpoint: packets=%+v qualified=%t", packets, p.markerQualified)
	})
}
