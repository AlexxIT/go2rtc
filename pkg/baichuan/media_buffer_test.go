package baichuan

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

func assemblyVideo(keyframe bool, codec string, payload int) []byte {
	extra, magic := 0, uint32(mediaPFrameMin)
	if keyframe {
		extra, magic = 4, mediaIFrameMin
	}
	dataStart := mediaVideoHeaderSize + extra
	wire := make([]byte, dataStart+payload+int(padding(uint32(payload))))
	binary.LittleEndian.PutUint32(wire, magic)
	copy(wire[4:], codec)
	binary.LittleEndian.PutUint32(wire[8:], uint32(payload))
	binary.LittleEndian.PutUint32(wire[12:], uint32(extra))
	for i := range wire[dataStart : dataStart+payload] {
		wire[dataStart+i] = byte(i%127 + 1)
	}
	return wire
}

func TestMediaFragmentReserveOwnership(t *testing.T) {
	for _, keyframe := range []bool{false, true} {
		for _, codec := range []string{"H264", "H265"} {
			t.Run(fmt.Sprintf("%s/%t", codec, keyframe), func(t *testing.T) {
				frame := assemblyVideo(keyframe, codec, (96<<10)+3)
				dataStart := mediaVideoHeaderSize + int(binary.LittleEndian.Uint32(frame[12:]))
				want := bytes.Clone(frame[dataStart : dataStart+(96<<10)+3])
				wire := append(bytes.Clone(frame), infoFixture()...)
				p, _ := NewMediaParser(Limits{})
				if packets, err := p.appendOwned(bytes.Clone(wire[:23])); err != nil || len(packets) != 0 || cap(p.buf) >= 32<<10 {
					t.Fatal("partial header reserved frame storage", err)
				}
				if packets, err := p.appendOwned(bytes.Clone(wire[23 : 32<<10])); err != nil || len(packets) != 0 ||
					len(p.buf) != 32<<10 || cap(p.buf) < len(frame) || cap(p.buf) > 2<<20 {
					t.Fatal("validated incomplete frame was not reserved without consuming bytes", err)
				}
				storage := p.buf
				var packets []MediaPacket
				for offset := 32 << 10; offset < len(wire); {
					end := min(offset+39850, len(wire))
					next, err := p.appendOwned(bytes.Clone(wire[offset:end]))
					if err != nil {
						t.Fatal(err)
					}
					packets = append(packets, next...)
					offset = end
				}
				if len(packets) != 2 || packets[1].Kind != MediaInfo || !bytes.Equal(packets[0].Data, want) ||
					&packets[0].Data[0] != &storage[dataStart] {
					t.Fatal("reservation changed payload, padding, or completed-frame ownership")
				}
				if _, err := p.appendOwned(append(recoveryPFixture(codec), infoFixture()...)); err != nil || !bytes.Equal(packets[0].Data, want) {
					t.Fatal("later feeds changed the retained completed frame", err)
				}
				owned := bytes.Clone(wire)
				p, _ = NewMediaParser(Limits{})
				complete, err := p.appendOwned(owned)
				if err != nil || len(complete) != 2 || &complete[0].Data[0] != &owned[dataStart] {
					t.Fatal("complete frame lost zero-copy adoption", err)
				}
			})
		}
	}
}

func TestMediaFragmentReserveHintBounds(t *testing.T) {
	for _, keyframe := range []bool{false, true} {
		p, _ := NewMediaParser(Limits{})
		frame := assemblyVideo(keyframe, "H265", 2<<20)
		if _, size, complete, err := p.parsePacket(frame[:23]); err != nil || size != 0 || complete {
			t.Fatal("partial header supplied an allocation hint", err)
		}
		if packets, err := p.appendOwned(bytes.Clone(frame[:24])); err != nil || len(packets) != 0 || cap(p.buf) >= 32<<10 {
			t.Fatal("tiny validated header eagerly allocated a large frame", err)
		}
		if packets, err := p.appendOwned(bytes.Clone(frame[24 : 32<<10])); err != nil || len(packets) != 0 || cap(p.buf) >= 2<<20 {
			t.Fatal("frame above the fast-path limit was reserved", err)
		}
		for _, codec := range []string{"BAD!", "H265"} {
			header := bytes.Clone(frame[:24])
			copy(header[4:], codec)
			binary.LittleEndian.PutUint32(header[8:], ^uint32(0))
			binary.LittleEndian.PutUint32(header[12:], ^uint32(0))
			if _, size, _, err := p.parsePacket(header); err == nil || size != 0 {
				t.Fatal("invalid codec or overflowing declared size supplied a hint", err)
			}
		}
	}
}
