package miss

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
)

type testFrame struct{ header, payload []byte }

type testConn struct {
	frames []testFrame
	writes [][]byte
}

func (c *testConn) Protocol() string                     { return "test" }
func (c *testConn) Version() string                      { return "test" }
func (c *testConn) ReadCommand() (uint32, []byte, error) { return 0, nil, io.EOF }
func (c *testConn) WriteCommand(_ uint32, data []byte) error {
	c.writes = append(c.writes, append([]byte(nil), data...))
	return nil
}
func (c *testConn) ReadPacket() ([]byte, []byte, error) {
	if len(c.frames) == 0 {
		return nil, nil, io.EOF
	}
	f := c.frames[0]
	c.frames = c.frames[1:]
	return f.header, f.payload, nil
}
func (c *testConn) WritePacket(_, _ []byte) error { return nil }
func (c *testConn) RemoteAddr() net.Addr          { return &net.UDPAddr{} }
func (c *testConn) SetDeadline(time.Time) error   { return nil }
func (c *testConn) Close() error                  { return nil }

func TestDialErrorReturnsNilProducer(t *testing.T) {
	if p, err := Dial("%"); err == nil || p != nil {
		t.Errorf("Dial returned %T, %v", p, err)
	}

	var pair LensPair
	if p, err := pair.Open(true, func() (string, error) { return "", io.EOF }); err != io.EOF || p != nil {
		t.Fatalf("Open returned %T, %v", p, err)
	}
}

func TestDualLensStartMedia(t *testing.T) {
	for _, audio := range []string{"0", "1"} {
		conn := &testConn{}
		client := &Client{Conn: conn, key: make([]byte, 32), model: "xiaomi.camera.082ac1"}
		if err := client.startMedia("", "3", audio, true); err != nil {
			t.Fatal(err)
		}
		if len(conn.writes) != 1 {
			t.Fatalf("wanted one media-start command, got %d", len(conn.writes))
		}
		data, err := crypto.Decode(conn.writes[0], client.key)
		if err != nil {
			t.Fatal(err)
		}
		if binary.BigEndian.Uint32(data) != cmdVideoStart {
			t.Fatal("wrong media command")
		}
		var body map[string]int
		if err = json.Unmarshal(data[4:], &body); err != nil {
			t.Fatal(err)
		}
		if body["videoquality"] != 3 || body["videoquality2"] != 3 || body["enableaudio"] != int(audio[0]-'0') {
			t.Fatalf("unexpected dual media request: %v", body)
		}
	}
}

func makeTestFrame(t *testing.T, key []byte, codec, flags, sequence uint32, data []byte) testFrame {
	t.Helper()
	payload, err := crypto.Encode(data, key)
	if err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header, uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[4:], codec)
	binary.LittleEndian.PutUint32(header[8:], sequence)
	binary.LittleEndian.PutUint32(header[12:], flags)
	return testFrame{header, payload}
}

func TestDualLensProbeAndRouting(t *testing.T) {
	for _, video := range []struct {
		name       string
		codec      uint32
		parameters string
		key, pred  byte
	}{
		{"H264", codecH264, "000000016764001fac2484014016ec0440000003004000000c23c60c92", 0x65, 0x41},
		{"H265", codecH265, "0000000140010c01", 0x26, 0x02},
	} {
		t.Run(video.name, func(t *testing.T) {
			key := make([]byte, 32)
			// Deliberately identical parameter sets for both lenses: resolution and codec cannot identify a lens.
			sps, _ := hex.DecodeString(video.parameters)
			conn := &testConn{frames: []testFrame{
				makeTestFrame(t, key, video.codec, 0x001e2001, 1, sps),
				makeTestFrame(t, key, video.codec, 0x015e2001, 2, sps),
				makeTestFrame(t, key, codecOPUS, 0, 3, []byte{1}),
			}}
			client := &Client{Conn: conn, key: key, model: "xiaomi.camera.082ac1"}
			medias, err := probe(client, true, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(medias) != 4 || medias[0].Kind != core.KindVideo || medias[1].Kind != core.KindVideo {
				t.Fatalf("expected two video tracks and shared audio/backchannel: %v", medias)
			}
			if medias[0].Codecs[0] == medias[1].Codecs[0] || medias[0].Codecs[0].FmtpLine != medias[1].Codecs[0].FmtpLine {
				t.Fatal("same-format lenses need distinct track identities")
			}
			producer := &Producer{Connection: core.Connection{Medias: medias}, client: client}
			var pair LensPair
			first, err := pair.open(0, func() (*Producer, error) { return producer, nil })
			if err != nil {
				t.Fatal(err)
			}
			second, err := pair.open(1, func() (*Producer, error) { t.Fatal("second device dial"); return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Medias) != 3 || first.Medias[2].Direction != core.DirectionSendonly || first.Medias[2] != second.Medias[2] || len(conn.writes) != 0 {
				t.Fatal("forward open changed the shared speaker")
			}
			producer.lenses[0].state.Store(lensWaitKey)
			var received [4][]uint16
			for i, r := range []*core.Receiver{first.Receivers[0], second.Receivers[0], first.Receivers[1], second.Receivers[1]} {
				r.Input = func(packet *core.Packet) {
					received[i] = append(received[i], packet.SequenceNumber)
					if i >= 2 {
						if !packet.Marker {
							t.Error("microphone header changed by the other stream")
						}
						packet.Marker = false
					}
					if i == 0 {
						producer.lenses[1].state.Store(lensWaitKey)
					}
				}
			}
			conn.frames = []testFrame{
				makeTestFrame(t, key, video.codec, 0x015e2000, 8, []byte{0, 0, 0, 1, video.key, 2}), // before second view joins
				makeTestFrame(t, key, video.codec, 0x001e2000, 9, []byte{0, 0, 0, 1, video.pred, 1}),
				makeTestFrame(t, key, video.codec, 0x001e2000, 10, []byte{0, 0, 0, 1, video.key, 1}),
				makeTestFrame(t, key, video.codec, 0x015e2000, 19, []byte{0, 0, 0, 1, video.pred, 2}),
				makeTestFrame(t, key, video.codec, 0x015e2000, 20, []byte{0, 0, 0, 1, video.key, 2}),
				makeTestFrame(t, key, codecOPUS, 0, 30, []byte{1}),
			}
			_ = producer.Start() // finite fake transport finishes with EOF
			for i, sequence := range []uint16{10, 20, 30, 30} {
				if len(received[i]) != 1 || received[i][0] != sequence {
					t.Fatalf("track %d received other lens or audio: %v", i, received[i])
				}
			}
		})
	}
}

func TestDualLensProbeRequiresBothLenses(t *testing.T) {
	key := make([]byte, 32)
	sps, _ := hex.DecodeString("000000016764001fac2484014016ec0440000003004000000c23c60c92")
	conn := &testConn{frames: []testFrame{makeTestFrame(t, key, codecH264, 0, 1, sps)}}
	_, err := probe(&Client{Conn: conn, key: key}, false, 2)
	if err == nil || !strings.Contains(err.Error(), "missing video channel") {
		t.Fatalf("one lens must not pass dual-lens discovery: %v", err)
	}
}

// A failed reader must release the old slot before either view reconnects.
// Counters and encoded payloads here are synthetic.
type pairWire struct {
	testConn
	read     chan struct{}
	closedCh chan struct{}
}

func (w *pairWire) ReadPacket() ([]byte, []byte, error) {
	close(w.read)
	<-w.closedCh
	return nil, nil, io.EOF
}

func (w *pairWire) Close() error { close(w.closedCh); return nil }

func TestLensPairLifecycle(t *testing.T) {
	var pair LensPair
	var wires []*pairWire
	dial := func() (*Producer, error) {
		w := &pairWire{read: make(chan struct{}), closedCh: make(chan struct{})}
		wires = append(wires, w)
		makeMedia := func(kind, name string) *core.Media {
			return &core.Media{Kind: kind, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: name}}}
		}
		c := &Client{Conn: w, key: make([]byte, 32)}
		return &Producer{Connection: core.Connection{Medias: []*core.Media{makeMedia(core.KindVideo, core.CodecH265), makeMedia(core.KindVideo, core.CodecH265), makeMedia(core.KindAudio, core.CodecOpus)}, Transport: c}, client: c}, nil
	}
	open := func(channel int) *lens {
		t.Helper()
		v, err := pair.open(channel, dial)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	second, first := open(1), open(0)
	if _, err := pair.open(1, dial); err == nil {
		t.Fatal("independent producers acquired the same lens")
	}
	if len(wires) != 1 || second.GetMedias()[0] == first.GetMedias()[0] || second.GetMedias()[1] != first.GetMedias()[1] || second.Receivers[1] == first.Receivers[1] {
		t.Fatal("invalid lens or shared microphone identity")
	}
	ended := make(chan error, 2)
	go func() { ended <- second.Start() }()
	go func() { ended <- first.Start() }()
	<-wires[0].read
	if err := second.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := <-ended; err != nil {
		t.Fatal(err)
	}
	select {
	case <-wires[0].closedCh:
		t.Fatal("closing the second view closed the camera")
	default:
	}
	second = open(1)
	go func() { ended <- second.Start() }()
	_ = first.session.camera.client.Close() // simulate transport loss while both views are subscribed
	for range 2 {
		if err := <-ended; !errors.Is(err, io.EOF) {
			t.Fatalf("reader returned %v", err)
		}
	}
	newSecond, newFirst := open(1), open(0)
	if len(wires) != 2 || newSecond.session != newFirst.session {
		t.Fatal("reconnect opened multiple device slots")
	}
	_ = second.Stop()
	_ = first.Stop()
	select {
	case <-wires[1].closedCh:
		t.Fatal("old view closed its replacement")
	default:
	}
	_ = newFirst.Stop()
	_ = newSecond.Stop()
	_ = newSecond.Stop()
	select {
	case <-wires[1].closedCh:
	default:
		t.Fatal("last view did not release the device")
	}
}
