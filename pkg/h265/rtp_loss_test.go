package h265

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestRTPDepayIntactAccessUnit(t *testing.T) {
	var got [][]byte
	depay := RTPDepay(&core.Codec{}, func(p *rtp.Packet) { got = append(got, bytes.Clone(p.Payload)) })
	// Include sequence rollover and the marked-SEI camera compatibility path.
	depay(lossPacket(65533, 100, true, []byte{78, 1, 5}))
	depay(lossPacket(65534, 99, true, []byte{64, 1, 7}))
	depay(lossPacket(65535, 100, false, []byte{98, 1, 147, 10}))
	depay(lossPacket(0, 100, false, []byte{98, 1, 19, 11}))
	depay(lossPacket(1, 100, true, []byte{98, 1, 83, 12}))
	want := lossNALs([]byte{64, 1, 7}, []byte{38, 1, 10, 11, 12})
	if len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestRTPDepayLossWaitsForCompleteKeyframe(t *testing.T) {
	tests := []struct {
		name    string
		packets []*rtp.Packet
	}{
		{"missing middle fragment", []*rtp.Packet{
			lossPacket(10, 100, false, []byte{98, 1, 129, 10}),
			lossPacket(12, 100, true, []byte{98, 1, 65, 12}),
		}},
		{"missing start fragment", []*rtp.Packet{
			lossPacket(10, 100, false, []byte{98, 1, 1, 10}),
			lossPacket(11, 100, true, []byte{98, 1, 65, 11}),
		}},
		{"timestamp changes before end", []*rtp.Packet{
			lossPacket(10, 100, false, []byte{98, 1, 129, 10}),
			lossPacket(11, 200, true, []byte{98, 1, 65, 11}),
		}},
		{"new start before end", []*rtp.Packet{
			lossPacket(10, 100, false, []byte{98, 1, 129, 10}),
			lossPacket(11, 100, false, []byte{98, 1, 129, 11}),
			lossPacket(12, 100, true, []byte{98, 1, 65, 12}),
		}},
		{"whole frame lost between access units", []*rtp.Packet{
			lossPacket(10, 100, true, []byte{2, 1, 10}),
			lossPacket(12, 300, true, []byte{2, 1, 12}),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][]byte
			depay := RTPDepay(&core.Codec{}, func(p *rtp.Packet) { got = append(got, bytes.Clone(p.Payload)) })
			for _, p := range tt.packets {
				depay(p)
			}
			if tt.name == "whole frame lost between access units" {
				if len(got) < 1 || !bytes.Equal(got[0], lossNALs([]byte{2, 1, 10})) {
					t.Fatal("intact initial frame lost")
				}
				got = got[1:]
			}
			seq := tt.packets[len(tt.packets)-1].SequenceNumber
			send := func(ts uint32, marker bool, b []byte) { seq++; depay(lossPacket(seq, ts, marker, b)) }
			send(400, true, []byte{2, 1, 13})
			if len(got) != 0 {
				t.Fatalf("forwarded damaged or dependent frames: %x", got)
			}
			// Losing a piece of the next keyframe must not release dependent frames.
			send(500, false, []byte{98, 1, 147, 14})
			seq++
			send(500, true, []byte{98, 1, 83, 16})
			send(600, true, []byte{2, 1, 17})
			if len(got) != 0 {
				t.Fatal("resumed on incomplete keyframe")
			}
			send(700, false, []byte{64, 1, 7})
			send(700, false, []byte{98, 1, 147, 18})
			send(700, true, []byte{98, 1, 83, 19})
			send(800, true, []byte{2, 1, 20})
			want := lossNALs([]byte{64, 1, 7}, []byte{38, 1, 18, 19})
			if len(got) != 2 || !bytes.Equal(got[0], want) || !bytes.Equal(got[1], lossNALs([]byte{2, 1, 20})) {
				t.Fatalf("bad recovery: %x", got)
			}
		})
	}
}

func lossPacket(seq uint16, ts uint32, marker bool, data []byte) *rtp.Packet {
	return &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts, Marker: marker}, Payload: data}
}
func lossNALs(nals ...[]byte) []byte {
	var b []byte
	for _, n := range nals {
		b = binary.BigEndian.AppendUint32(b, uint32(len(n)))
		b = append(b, n...)
	}
	return b
}

func TestRTPDepayEveryFragmentLoss(t *testing.T) {
	var packets []*rtp.Packet
	var expected [][]byte
	for frame := 0; frame < 8; frame++ {
		typ := byte(1)
		if frame%4 == 0 {
			typ = 19
		}
		expected = append(expected, lossNALs([]byte{typ << 1, 1, byte(frame), 10, 11}))
		packets = append(packets,
			lossPacket(uint16(len(packets)), uint32(frame*3000), false, []byte{98, 1, 128 | typ, byte(frame)}),
			lossPacket(uint16(len(packets)+1), uint32(frame*3000), false, []byte{98, 1, typ, 10}),
			lossPacket(uint16(len(packets)+2), uint32(frame*3000), true, []byte{98, 1, 64 | typ, 11}),
		)
	}
	for missing := 3; missing < 21; missing++ {
		var got [][]byte
		depay := RTPDepay(&core.Codec{}, func(p *rtp.Packet) { got = append(got, bytes.Clone(p.Payload)) })
		for i, p := range packets {
			if i != missing {
				depay(p)
			}
		}
		lostFrame := missing / 3
		nextKey := (lostFrame/4 + 1) * 4
		var want [][]byte
		for frame, b := range expected {
			if frame < lostFrame || frame >= nextKey {
				want = append(want, b)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("missing packet %d: got %d frames, want %d", missing, len(got), len(want))
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("missing packet %d: frame %d corrupted", missing, i)
			}
		}
	}
}

func TestRTPDepayPreservesSDPParameterSets(t *testing.T) {
	var got []byte
	codec := &core.Codec{FmtpLine: "sprop-vps=QAEH;sprop-sps=QgEI;sprop-pps=RAEJ"}
	depay := RTPDepay(codec, func(p *rtp.Packet) { got = bytes.Clone(p.Payload) })
	depay(lossPacket(10, 100, false, []byte{98, 1, 147, 10}))
	depay(lossPacket(11, 100, true, []byte{98, 1, 83, 11}))
	want := lossNALs([]byte{64, 1, 7}, []byte{66, 1, 8}, []byte{68, 1, 9}, []byte{38, 1, 10, 11})
	if !bytes.Equal(got, want) {
		t.Fatalf("parameter sets changed: got %x, want %x", got, want)
	}
}
