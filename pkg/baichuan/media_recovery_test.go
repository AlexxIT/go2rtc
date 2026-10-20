package baichuan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMediaParserRecoversRejectedVideoHeader(t *testing.T) {
	for _, magic := range []uint32{mediaIFrameMin, mediaPFrameMin} {
		for _, test := range []struct {
			name  string
			field int
			value uint32
		}{
			{"codec", 4, 0}, {"payload", 8, ^uint32(0)},
			{"extra", 12, 1 << 20}, {"extra-sign", 12, 1 << 31}, {"extra-max", 12, ^uint32(0)},
		} {
			t.Run(fmt.Sprintf("%x/%s", magic, test.name), func(t *testing.T) {
				bad := recoveryPFixture("H264")[:mediaVideoHeaderSize]
				binary.LittleEndian.PutUint32(bad, magic)
				binary.LittleEndian.PutUint32(bad[test.field:], test.value)
				p, _ := NewMediaParser(Limits{MaxMediaFrame: 128, MaxMediaBuffer: 256, MaxResync: 64})
				decoy := recoveryPFixture("H264")
				decoy[mediaVideoHeaderSize] = 1 // Invalid Annex B prefix.
				wire := append(append(bad, decoy...), recoveryPFixture("H265")...)
				if packets, err := p.Append(wire[:len(wire)-5]); err != nil || len(packets) != 0 {
					t.Fatalf("fragment: packets=%+v err=%v", packets, err)
				}
				packets, err := p.Append(wire[len(wire)-5:])
				if err != nil || len(packets) != 2 || packets[0].Kind != MediaDiscontinuity || packets[1].Codec != "H265" {
					t.Fatalf("recovery: packets=%+v err=%v", packets, err)
				}
			})
		}
	}
}

func TestMediaParserRejectedVideoWindow(t *testing.T) {
	for _, resync := range []uint32{64, defaultMaxResync} {
		window := int(resync)
		if resync == defaultMaxResync {
			window = mediaRecoveryWindow
		}
		for _, beyond := range []int{0, 1} {
			p, _ := NewMediaParser(Limits{MaxResync: resync})
			offset := mediaVideoHeaderSize + window + beyond
			wire := make([]byte, mediaVideoHeaderSize+window+mediaRecoveryLookahead)
			binary.LittleEndian.PutUint32(wire, mediaPFrameMin)
			copy(wire[4:8], "nope")
			copy(wire[offset:], recoveryPFixture("H264"))
			copy(wire[offset+32:], infoFixture())
			packets, err := p.Append(wire)
			if beyond == 0 {
				if err != nil || len(packets) != 3 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaVideoP || packets[2].Kind != MediaInfo {
					t.Fatalf("edge %d: packets=%+v err=%v", window, packets, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "unsupported video codec") || len(packets) != 0 {
				t.Fatalf("beyond %d: packets=%+v err=%v", window, packets, err)
			}
		}
	}
}

func TestMediaParserWaitsForLegalVideoDeclaration(t *testing.T) {
	for _, magic := range []uint32{mediaIFrameMin, mediaPFrameMin} {
		wire := make([]byte, 256)
		binary.LittleEndian.PutUint32(wire, magic)
		copy(wire[4:8], "H264")
		binary.LittleEndian.PutUint32(wire[8:12], uint32(len(wire)-mediaVideoHeaderSize))
		copy(wire[mediaVideoHeaderSize:], recoveryPFixture("H264"))
		p, _ := NewMediaParser(Limits{})
		if packets, err := p.Append(wire[:128]); err != nil || len(packets) != 0 {
			t.Fatalf("unfinished legal %x: packets=%+v err=%v", magic, packets, err)
		}
		packets, err := p.Append(append(wire[128:], infoFixture()...))
		if err != nil || len(packets) != 2 || len(packets[0].Data) != len(wire)-mediaVideoHeaderSize || packets[1].Kind != MediaInfo {
			t.Fatalf("legal %x: packets=%+v err=%v", magic, packets, err)
		}
	}
}

func pWithCompleteAACAtEnd() []byte {
	b := brokenPWithAAC()
	binary.LittleEndian.PutUint32(b[8:12], 37)
	return b[:72]
}

func keyframeBoundary(offset int, codec []byte) []byte {
	var payload []byte
	switch string(codec) {
	case "H264":
		payload = []byte{0, 0, 0, 1, 1}
	case "H265":
		payload = []byte{0, 0, 0, 1, 2, 1}
	}
	b := make([]byte, offset+mediaVideoHeaderSize+len(payload)+int(padding(uint32(len(payload)))))
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[offset:], mediaPFrameMin)
	copy(b[offset+4:offset+8], codec)
	binary.LittleEndian.PutUint32(b[offset+8:offset+12], uint32(len(payload)))
	copy(b[offset+mediaVideoHeaderSize:], payload)
	return b
}

func alignedPWithAAC() []byte {
	b := brokenPWithAAC()
	binary.LittleEndian.PutUint32(b[8:12], 32)
	fixture := append([]byte(nil), b[:40]...)
	fixture = append(fixture, b[48:72]...)

	next := make([]byte, mediaVideoHeaderSize+8)
	binary.LittleEndian.PutUint32(next, mediaPFrameMin)
	copy(next[4:8], "H264")
	binary.LittleEndian.PutUint32(next[8:12], 8)
	copy(next[mediaVideoHeaderSize:], []byte{0, 0, 0, 1, 1, 6, 7, 8})
	fixture = append(fixture, next...)
	return append(fixture, infoFixture()...)
}

func TestMediaParserRecoversAlignedPBoundaryBeforeAAC(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(alignedPWithAAC())
	if err != nil || len(packets) != 4 || packets[0].Kind != MediaDiscontinuity ||
		packets[1].Kind != MediaAAC || len(packets[1].Data) != 13 ||
		packets[2].Kind != MediaVideoP || packets[2].Codec != "H264" || len(packets[2].Data) != 8 ||
		packets[3].Kind != MediaInfo {
		t.Fatalf("incorrect aligned recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserRejectsInvalidAlignedAACAnchor(t *testing.T) {
	fixture := alignedPWithAAC()
	binary.LittleEndian.PutUint16(fixture[44:46], 12)
	binary.LittleEndian.PutUint16(fixture[46:48], 12)
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(fixture)
	if err != nil || len(packets) != 3 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaVideoP || packets[2].Kind != MediaInfo {
		t.Fatalf("did not skip invalid aligned AAC anchor: packets=%+v err=%v", packets, err)
	}
}

func TestMediaBoundaryAlignedLookaheadCompatibility(t *testing.T) {
	for _, length := range []int{32, 33, 35} {
		b := make([]byte, length)
		size, complete, err := mediaBoundary(b, 32, 32)
		if err != nil || !complete || size != 32 {
			t.Fatalf("changed packet-end boundary at length %d: size=%d complete=%t err=%v", length, size, complete, err)
		}
	}

	b := make([]byte, 32+len(infoFixture()))
	copy(b[32:], infoFixture())
	size, complete, err := mediaBoundary(b, 32, 32)
	if err != nil || !complete || size != 32 {
		t.Fatalf("changed valid aligned successor: size=%d complete=%t err=%v", size, complete, err)
	}
}

func TestMediaParserDefersCorroboratedPRecoveryUntilHeaderCompletes(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	fixture := pWithCompleteAACAtEnd()
	if packets, err := p.Append(fixture); err != nil || len(packets) != 0 {
		t.Fatalf("did not defer complete AAC candidate: packets=%+v err=%v", packets, err)
	}
	info := infoFixture()
	if packets, err := p.Append(info[:6]); err != nil || len(packets) != 0 {
		t.Fatalf("did not defer partial following header: packets=%+v err=%v", packets, err)
	}
	packets, err := p.Append(info[6:])
	if err != nil || len(packets) != 3 || packets[0].Kind != MediaDiscontinuity ||
		packets[1].Kind != MediaAAC || packets[2].Kind != MediaInfo {
		t.Fatalf("incorrect deferred recovery result: packets=%+v err=%v", packets, err)
	}

	fixture = brokenPWithAAC()
	binary.LittleEndian.PutUint32(fixture[8:12], 37)
	p, _ = NewMediaParser(Limits{})
	if packets, err := p.Append(fixture[:68]); err != nil || len(packets) != 0 {
		t.Fatalf("did not defer validated AAC with partial body: packets=%+v err=%v", packets, err)
	}
	if packets, err := p.Append(fixture[68:72]); err != nil || len(packets) != 0 {
		t.Fatalf("did not defer AAC until successor arrives: packets=%+v err=%v", packets, err)
	}
	packets, err = p.Append(fixture[72:])
	if err != nil || len(packets) != 4 || packets[0].Kind != MediaDiscontinuity ||
		packets[1].Kind != MediaAAC || packets[2].Kind != MediaInfo || packets[3].Kind != MediaInfo {
		t.Fatalf("incorrect fragmented AAC recovery: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserRejectsInvalidPRecoverySuccessor(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	if packets, err := p.Append(pWithCompleteAACAtEnd()); err != nil || len(packets) != 0 {
		t.Fatalf("did not defer complete AAC candidate: packets=%+v err=%v", packets, err)
	}
	if packets, err := p.Append([]byte{0}); len(packets) != 0 || err != nil {
		t.Fatalf("accepted invalid following bytes: packets=%+v err=%v", packets, err)
	}
	packets, err := p.Append(infoFixture())
	if err != nil || len(packets) != 2 || packets[0].Kind != MediaDiscontinuity || packets[1].Kind != MediaInfo {
		t.Fatalf("did not skip uncorroborated audio: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserExtendsOnlyDefaultForwardKeyframeWindow(t *testing.T) {
	const frameLimit = 256 << 10
	offset := mediaVideoHeaderSize + 2*int(defaultMaxResync)
	limits, err := (Limits{MaxMediaFrame: frameLimit, MaxMediaBuffer: 512 << 10}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	fixture := keyframeBoundary(offset, []byte("H264"))
	const split = 8
	p, err := NewMediaParser(limits)
	if err != nil {
		t.Fatal(err)
	}
	if packets, err := p.Append(fixture[:offset+split]); err != nil || len(packets) != 0 {
		t.Fatalf("rejected upper-edge split: packets=%+v err=%v", packets, err)
	}
	packets, err := p.Append(fixture[offset+split:])
	if err != nil || len(packets) != 2 || packets[0].Kind != MediaVideoI || packets[1].Kind != MediaVideoP {
		t.Fatalf("failed 128 KiB split: packets=%+v err=%v", packets, err)
	}

	const customResync = 1024
	p, err = NewMediaParser(Limits{MaxMediaFrame: frameLimit, MaxMediaBuffer: 512 << 10, MaxResync: customResync})
	if err != nil {
		t.Fatal(err)
	}
	customOffset := mediaVideoHeaderSize + customResync + 1
	customFixture := keyframeBoundary(customOffset, []byte("H264"))
	customFixture = append(customFixture, make([]byte, mediaRecoveryLookahead-mediaVideoHeaderSize)...)
	packets, err = p.Append(customFixture)
	if !errors.Is(err, errNoKeyframeBoundary) || len(packets) != 0 {
		t.Fatalf("expanded an explicit custom boundary: packets=%+v err=%v", packets, err)
	}
}

func TestMediaParserRejectsWeakExtendedKeyframeBoundary(t *testing.T) {
	p, err := NewMediaParser(Limits{MaxMediaFrame: 256 << 10, MaxMediaBuffer: 512 << 10})
	if err != nil {
		t.Fatal(err)
	}
	offset := mediaVideoHeaderSize + 2*int(defaultMaxResync)
	fixture := keyframeBoundary(offset, []byte("nope"))
	fixture = append(fixture, make([]byte, (256<<10)+mediaRecoveryLookahead-len(fixture))...)
	if packets, err := p.Append(fixture); !errors.Is(err, errNoKeyframeBoundary) || len(packets) != 0 {
		t.Fatalf("accepted weak extended boundary: packets=%+v err=%v", packets, err)
	}
}
