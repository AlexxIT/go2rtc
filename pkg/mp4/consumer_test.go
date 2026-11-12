package mp4

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/av1"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// 3840x2160 Main profile level 5.0 10-bit, the configOBUs of an av1C record
// written by ffmpeg. Deliberately not 1920x1080 Main 4.0 8-bit: those are the
// placeholder values, so a test using them cannot tell a parsed sequence header
// from no sequence header at all.
var testSeqHdr = []byte{
	0x0A, 0x0C, 0x00, 0x00, 0x00, 0x62, 0xEF, 0xBF, 0xE1, 0xBC, 0x02, 0xF8, 0x40,
	0x40,
}

var (
	testKeyframeOBU  = []byte{0x32, 0x01, 0x00}
	testInterOBU     = []byte{0x32, 0x01, 0x20}
	testKeyframeUnit = append(append([]byte{}, testSeqHdr...), testKeyframeOBU...)
)

// recWriter records every Write, so tests can tell "init segment sent" from
// "WriteTo is still waiting" and can inspect what was actually muxed.
type recWriter struct {
	mu     sync.Mutex
	writes [][]byte
	signal chan struct{}
}

func newRecWriter() *recWriter {
	return &recWriter{signal: make(chan struct{}, 1)}
}

func (w *recWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, append([]byte{}, p...))
	w.mu.Unlock()

	select {
	case w.signal <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (w *recWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.writes)
}

func (w *recWriter) bytes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	var n int
	for _, b := range w.writes {
		n += len(b)
	}
	return n
}

func (w *recWriter) at(i int) []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	if i >= len(w.writes) {
		return nil
	}
	return w.writes[i]
}

func addTrack(t *testing.T, cons *Consumer, name string) *core.Receiver {
	t.Helper()

	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionSendonly}
	codec := &core.Codec{Name: name, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
	track := core.NewReceiver(media, codec)
	if err := cons.AddTrack(media, nil, track); err != nil {
		t.Fatal(err)
	}
	return track
}

func writeTo(t *testing.T, cons *Consumer) (*recWriter, chan struct{}) {
	wr := newRecWriter()
	done := make(chan struct{})
	go func() {
		_, _ = cons.WriteTo(wr)
		close(done)
	}()
	return wr, done
}

func waitFor(t *testing.T, ch chan struct{}, why string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(why)
	}
}

func mustNot(t *testing.T, ch chan struct{}, why string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal(why)
	case <-time.After(100 * time.Millisecond):
	}
}

// H264 and H265 take their codec params from the SDP, so the init segment must
// go out right away instead of waiting for the first keyframe.
func TestInitNotDelayed(t *testing.T) {
	cons := NewConsumer(nil)
	addTrack(t, cons, core.CodecH264)

	wr, _ := writeTo(t, cons)
	waitFor(t, wr.signal, "init segment was not written before the first keyframe")
}

// A stream that never delivers a keyframe must not pin the WriteTo goroutine
// once the consumer is stopped.
func TestWriteToReturnsAfterStop(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	wr, done := writeTo(t, cons)

	// inter frame only, no sequence header
	track.WriteRTP(&rtp.Packet{Payload: testInterOBU})
	mustNot(t, wr.signal, "AV1 init segment was written without a sequence header")

	_ = cons.Stop()
	waitFor(t, done, "WriteTo did not return after Stop, goroutine leaked")
}

// The av1C box and the MSE content type must come from the sequence header of
// the first keyframe, not from the placeholder defaults.
func TestAV1InitFromKeyframe(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	contentType := make(chan string, 1)
	cons.OnInit = func(ct string) { contentType <- ct }

	wr, _ := writeTo(t, cons)
	mustNot(t, wr.signal, "AV1 init segment was written before the first keyframe")

	if w, h := av1.DecodeSequenceHeader(testSeqHdr); w != 3840 || h != 2160 {
		t.Fatalf("fixture parses as %dx%d, want 3840x2160", w, h)
	}

	track.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})

	select {
	case ct := <-contentType:
		if want := `video/mp4; codecs="av01.0.12M.10"`; ct != want {
			t.Errorf("content type = %q, want %q", ct, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnInit was not called after the first keyframe")
	}

	waitFor(t, wr.signal, "init segment was not written after the first keyframe")
}

// Codecs is read from the HTTP goroutine (internal/mp4) and from the HLS
// session while a track handler may be filling in the AV1 sequence header.
func TestCodecsRace(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	go func() {
		time.Sleep(time.Millisecond)
		track.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	}()

	for deadline := time.Now().Add(50 * time.Millisecond); time.Now().Before(deadline); {
		_ = ContentType(cons.Codecs())
	}

	if got := ContentType(cons.Codecs()); got != `video/mp4; codecs="av01.0.12M.10"` {
		t.Errorf("content type = %q", got)
	}
}

// Some encoders send the sequence header in its own temporal unit instead of
// repeating it in front of every keyframe.
func TestAV1SequenceHeaderInEarlierUnit(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	contentType := make(chan string, 1)
	cons.OnInit = func(ct string) { contentType <- ct }

	wr, _ := writeTo(t, cons)
	mustNot(t, wr.signal, "init segment was written before any codec params")

	track.WriteRTP(&rtp.Packet{Payload: testSeqHdr}) // no frame

	select {
	case ct := <-contentType:
		if want := `video/mp4; codecs="av01.0.12M.10"`; ct != want {
			t.Errorf("content type = %q, want %q", ct, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnInit was not called after the sequence header")
	}

	waitFor(t, wr.signal, "init segment was not written after the sequence header")

	// a keyframe with no sequence header of its own still has to be muxed
	before := wr.count()
	track.WriteRTP(&rtp.Packet{Payload: testKeyframeOBU})
	waitFor(t, wr.signal, "keyframe was not muxed")
	if wr.count() <= before {
		t.Error("keyframe produced no fragment")
	}
}

// A keyframe alone is not enough, the av1C box would carry placeholder values.
func TestAV1WaitsForSequenceHeader(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	wr, done := writeTo(t, cons)

	for i := 0; i < 5; i++ {
		track.WriteRTP(&rtp.Packet{Payload: testKeyframeOBU})
	}
	mustNot(t, wr.signal, "init segment was written without a sequence header")

	_ = cons.Stop()
	waitFor(t, done, "WriteTo did not return after Stop")
}

// A stream is not a slideshow: everything after the first keyframe has to be
// muxed too, not just the keyframes.
func TestAV1InterFramesAreMuxed(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	wr, _ := writeTo(t, cons)
	track.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	waitFor(t, wr.signal, "init segment was not written")

	// the write buffer switches to wr only once WriteTo reaches it, so poll
	before := wr.bytes()
	deadline := time.Now().Add(2 * time.Second)
	for i := 0; ; i++ {
		track.WriteRTP(&rtp.Packet{Payload: testInterOBU})
		if wr.bytes() > before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no inter frame was muxed after %d attempts", i+1)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The sync sample flag decides whether a browser can start or seek at all.
func TestAV1SampleFlags(t *testing.T) {
	m := &Muxer{}
	m.AddTrack(&core.Codec{Name: core.CodecAV1, ClockRate: 90000})

	key := m.GetPayload(0, &rtp.Packet{Payload: testKeyframeUnit})
	inter := m.GetPayload(0, &rtp.Packet{Payload: testInterOBU})

	var find func(atoms []any) *iso.AtomTfhd
	find = func(atoms []any) *iso.AtomTfhd {
		for _, atom := range atoms {
			switch v := atom.(type) {
			case []any:
				if tfhd := find(v); tfhd != nil {
					return tfhd
				}
			case *iso.AtomTfhd:
				return v
			}
		}
		return nil
	}

	flag := func(b []byte) uint32 {
		atoms, err := iso.DecodeAtoms(b)
		require.NoError(t, err, err)
		tfhd := find(atoms)
		require.NotNil(t, tfhd, "no tfhd box")
		return tfhd.SampleFlags
	}

	require.Equal(t, uint32(iso.SampleVideoIFrame), flag(key), "keyframe sample flags")
	require.Equal(t, uint32(iso.SampleVideoNonIFrame), flag(inter), "inter frame sample flags")
}

// The init segment has to describe the real track, not the placeholder.
func TestAV1InitSegmentBytes(t *testing.T) {
	m := &Muxer{}
	m.AddTrack(&core.Codec{Name: core.CodecAV1, ClockRate: 90000,
		FmtpLine: av1.EncodeFmtpLine(testSeqHdr)})

	init, err := m.GetInit()
	require.NoError(t, err, err)

	require.True(t, bytes.Contains(init, []byte("av01")), "no av01 sample entry")
	i := bytes.Index(init, []byte("av1C"))
	if i < 0 {
		t.Fatal("no av1C box")
	}
	require.True(t, bytes.HasPrefix(init[i+4:], av1.EncodeConfig(testSeqHdr)), "av1C box was not built from the sequence header")

	// the placeholder track size must not survive into the init segment
	placeholder := &Muxer{}
	placeholder.AddTrack(&core.Codec{Name: core.CodecAV1, ClockRate: 90000})
	if def, err := placeholder.GetInit(); err == nil && bytes.Equal(init, def) {
		t.Error("init segment is identical to the one built without a sequence header")
	}
}

// Two video tracks: the AV1 one must not be starved by the other one starting
// first, or WriteTo never gets its codec params.
func TestTwoVideoTracks(t *testing.T) {
	cons := NewConsumer(nil)

	// RTP payload type, so the packet goes through RTPDepay rather than
	// RepairAVCC, which rewrites the payload in place
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionSendonly}
	h264 := core.NewReceiver(media, &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96})
	if err := cons.AddTrack(media, nil, h264); err != nil {
		t.Fatal(err)
	}

	av1t := addTrack(t, cons, core.CodecAV1)

	wr, _ := writeTo(t, cons)

	// single NAL unit packet holding an IDR slice
	h264.WriteRTP(&rtp.Packet{Header: rtp.Header{Marker: true}, Payload: []byte{0x65, 0x88, 0x84, 0x00}})
	mustNot(t, wr.signal, "init segment was written before the AV1 codec params")

	av1t.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	waitFor(t, wr.signal, "AV1 track was starved by the H264 track")
}

// A func field without a json tag makes encoding/json refuse the whole
// consumer, which empties /api/streams for every user.
func TestConsumerJSON(t *testing.T) {
	if _, err := json.Marshal(NewConsumer(nil)); err != nil {
		t.Errorf("json.Marshal: %v", err)
	}
}

// frame.mp4 for AV1: the snapshot must be a complete MP4 and must not appear
// before a keyframe with a sequence header.
func TestKeyframeAV1(t *testing.T) {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionSendonly}
	codec := &core.Codec{Name: core.CodecAV1, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
	track := core.NewReceiver(media, codec)

	cons := NewKeyframe(nil)
	if err := cons.AddTrack(media, nil, track); err != nil {
		t.Fatal(err)
	}

	wr := newRecWriter()
	go func() { _, _ = cons.WriteTo(wr) }()

	track.WriteRTP(&rtp.Packet{Payload: testInterOBU})
	mustNot(t, wr.signal, "snapshot was written for an inter frame")

	track.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	waitFor(t, wr.signal, "no snapshot after the keyframe")

	b := wr.at(0)
	if !bytes.Contains(b, []byte("ftyp")) || !bytes.Contains(b, []byte("moov")) {
		t.Error("snapshot is missing the init segment")
	}
	// the box must carry the params of the sequence header, not the placeholder
	require.True(t, bytes.Contains(b, av1.EncodeConfig(testSeqHdr)), "av1C box was not built from the sequence header")
	if got := ContentType(cons.Codecs()); got != `video/mp4; codecs="av01.0.12M.10"` {
		t.Errorf("content type = %q, want the params of the sequence header", got)
	}
}

// The browser sends every AV1 profile it can decode, but go2rtc serves whatever
// the stream has, so they have to collapse into a single AV1 codec.
func TestParseCodecsAV1(t *testing.T) {
	medias := ParseCodecs("avc1.640029,av01.0.08M.08,av01.0.12M.08,av01.0.13M.10,mp4a.40.2", true)

	var video *core.Media
	for _, media := range medias {
		if media.Kind == core.KindVideo {
			video = media
		}
	}
	require.NotNil(t, video, "no video media")

	var names []string
	for _, codec := range video.Codecs {
		names = append(names, codec.Name)
	}
	want := []string{core.CodecH264, core.CodecAV1}
	require.Equal(t, len(want), len(names), "video codecs = %v, want %v", names, want)
	for i := range want {
		require.Equal(t, want[i], names[i], "video codecs = %v, want %v", names, want)
	}
}

// The sequence header alone must not open the gate: muxing has to start on a
// keyframe, or MSE gets handed a mid-GOP fragment first.
func TestAV1StartsOnKeyframeOnly(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	wr, _ := writeTo(t, cons)

	track.WriteRTP(&rtp.Packet{Payload: testSeqHdr}) // params, no frame
	waitFor(t, wr.signal, "init segment was not written after the sequence header")

	before := wr.count()
	for i := 0; i < 5; i++ {
		track.WriteRTP(&rtp.Packet{Payload: testInterOBU})
	}
	mustNot(t, wr.signal, "an inter frame was muxed before the first keyframe")
	require.Equal(t, before, wr.count(), "fragments were written before the first keyframe")

	track.WriteRTP(&rtp.Packet{Payload: testKeyframeOBU})
	waitFor(t, wr.signal, "the keyframe was not muxed")
}

// The pending counter exists for consumers with more than one AV1 track: the
// init segment may only be built once every track knows its params.
func TestTwoAV1Tracks(t *testing.T) {
	cons := NewConsumer(nil)
	first := addTrack(t, cons, core.CodecAV1)
	second := addTrack(t, cons, core.CodecAV1)

	wr, _ := writeTo(t, cons)

	first.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	mustNot(t, wr.signal, "init segment was built while a track still had no sequence header")

	second.WriteRTP(&rtp.Packet{Payload: testKeyframeUnit})
	waitFor(t, wr.signal, "init segment was not written after both tracks had their params")
}

// WaitInit has to give up when the consumer is stopped, even if the init became
// ready at the same moment.
func TestWaitInitAfterStop(t *testing.T) {
	cons := NewConsumer(nil)
	track := addTrack(t, cons, core.CodecAV1)

	track.WriteRTP(&rtp.Packet{Payload: testSeqHdr}) // closes initCh
	_ = cons.Stop()

	if cons.WaitInit(0) {
		t.Error("WaitInit reported success after Stop")
	}
}

// The HLS session cannot wait forever for a sequence header.
func TestWaitInitTimeout(t *testing.T) {
	cons := NewConsumer(nil)
	addTrack(t, cons, core.CodecAV1)
	t.Cleanup(func() { _ = cons.Stop() })

	start := time.Now()
	if cons.WaitInit(50 * time.Millisecond) {
		t.Error("WaitInit reported success without a sequence header")
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("WaitInit returned after %v, before the timeout", elapsed)
	}
}

// frame.mp4 for H264 is the common case: its codec params come from the SDP, so
// the init segment must be prepended to the snapshot without any waiting.
func TestKeyframeH264(t *testing.T) {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionSendonly}
	// RTP payload type, so the packet goes through RTPDepay rather than
	// RepairAVCC, which rewrites the payload in place
	codec := &core.Codec{
		Name: core.CodecH264, ClockRate: 90000, PayloadType: 96,
		FmtpLine: "profile-level-id=640029;sprop-parameter-sets=Z2QAKaxWgHgCJ+WEAAADAAQAAAMAyDxQqSo=,aO48sA==",
	}
	track := core.NewReceiver(media, codec)

	cons := NewKeyframe(nil)
	if err := cons.AddTrack(media, nil, track); err != nil {
		t.Fatal(err)
	}

	wr := newRecWriter()
	go func() { _, _ = cons.WriteTo(wr) }()
	t.Cleanup(func() { _ = cons.Stop() })

	// single NAL unit packet holding a non-IDR slice
	track.WriteRTP(&rtp.Packet{Header: rtp.Header{Marker: true}, Payload: []byte{0x41, 0x9A, 0x00, 0x00}})
	mustNot(t, wr.signal, "snapshot was written for a non-keyframe")

	// IDR slice
	track.WriteRTP(&rtp.Packet{Header: rtp.Header{Marker: true}, Payload: []byte{0x65, 0x88, 0x84, 0x00}})
	waitFor(t, wr.signal, "no snapshot after the keyframe")

	b := wr.at(0)
	if !bytes.Contains(b, []byte("ftyp")) || !bytes.Contains(b, []byte("moov")) {
		t.Error("snapshot is missing the init segment")
	}
	require.True(t, bytes.Contains(b, []byte("avcC")), "snapshot has no avcC box")
}
