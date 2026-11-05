package baichuan

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestMediaParserIFrame(t *testing.T) {
	t.Parallel()

	raw := new(bytes.Buffer)
	writeU32 := func(v uint32) {
		if err := binary.Write(raw, binary.LittleEndian, v); err != nil {
			t.Fatalf("binary.Write(): %v", err)
		}
	}

	writeU32(bcmediaIFrameMin)
	raw.WriteString("H264")
	writeU32(3)
	writeU32(4)
	writeU32(123)
	writeU32(0)
	writeU32(1700000000)
	raw.Write([]byte{0x01, 0x02, 0x03})
	raw.Write(make([]byte, 5))

	var parser MediaParser

	if packets, err := parser.Append(raw.Bytes()[:10]); err != nil || len(packets) != 0 {
		t.Fatalf("partial append = (%d packets, %v), want (0, nil)", len(packets), err)
	}

	packets, err := parser.Append(raw.Bytes()[10:])
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("Append() packets = %d, want 1", len(packets))
	}

	packet := packets[0]
	if packet.Kind != MediaPacketIFrame {
		t.Fatalf("packet.Kind = %v, want %v", packet.Kind, MediaPacketIFrame)
	}
	if packet.Codec != "H264" {
		t.Fatalf("packet.Codec = %q, want %q", packet.Codec, "H264")
	}
	if packet.TimestampMicrosecs != 123 {
		t.Fatalf("packet.TimestampMicrosecs = %d, want 123", packet.TimestampMicrosecs)
	}
	if !packet.HasTimestamp {
		t.Fatalf("packet.HasTimestamp = false, want true")
	}
	if !bytes.Equal(packet.Data, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("packet.Data = %v, want %v", packet.Data, []byte{0x01, 0x02, 0x03})
	}
	if packet.UnixTime == nil {
		t.Fatalf("packet.UnixTime is nil")
	}
	if got, want := packet.UnixTime.UTC(), time.Unix(1700000000, 0).UTC(); !got.Equal(want) {
		t.Fatalf("packet.UnixTime = %v, want %v", got, want)
	}
}

func TestMediaParserAACVariants(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		magic uint32
	}{
		{name: "bw50", magic: bcmediaAAC},
		{name: "bw51", magic: bcmediaAACV2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := []byte{0xff, 0xf9, 0x60, 0x40, 0x01}
			raw := new(bytes.Buffer)
			writeU32 := func(v uint32) {
				if err := binary.Write(raw, binary.LittleEndian, v); err != nil {
					t.Fatalf("binary.Write(): %v", err)
				}
			}
			writeU16 := func(v uint16) {
				if err := binary.Write(raw, binary.LittleEndian, v); err != nil {
					t.Fatalf("binary.Write(): %v", err)
				}
			}

			writeU32(tc.magic)
			writeU16(uint16(len(payload)))
			writeU16(uint16(len(payload)))
			raw.Write(payload)
			raw.Write(make([]byte, padLen(len(payload))))

			var parser MediaParser
			packets, err := parser.Append(raw.Bytes())
			if err != nil {
				t.Fatalf("Append() error = %v", err)
			}
			if len(packets) != 1 {
				t.Fatalf("Append() packets = %d, want 1", len(packets))
			}
			if packets[0].Kind != MediaPacketAAC {
				t.Fatalf("packet.Kind = %v, want %v", packets[0].Kind, MediaPacketAAC)
			}
			if packets[0].HasTimestamp {
				t.Fatalf("packet.HasTimestamp = true, want false")
			}
			if !bytes.Equal(packets[0].Data, payload) {
				t.Fatalf("packet.Data = %v, want %v", packets[0].Data, payload)
			}
		})
	}
}

func TestMediaParserADPCMNoTimestamp(t *testing.T) {
	t.Parallel()

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	payloadSize := 4 + len(payload)
	raw := new(bytes.Buffer)
	writeU32 := func(v uint32) {
		if err := binary.Write(raw, binary.LittleEndian, v); err != nil {
			t.Fatalf("binary.Write(): %v", err)
		}
	}
	writeU16 := func(v uint16) {
		if err := binary.Write(raw, binary.LittleEndian, v); err != nil {
			t.Fatalf("binary.Write(): %v", err)
		}
	}

	writeU32(bcmediaADPCM)
	writeU16(uint16(payloadSize))
	writeU16(uint16(payloadSize))
	writeU16(bcmediaADPCMHeader)
	writeU16(2)
	raw.Write(payload)

	var parser MediaParser
	packets, err := parser.Append(raw.Bytes())
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("Append() packets = %d, want 1", len(packets))
	}
	if packets[0].Kind != MediaPacketADPCM {
		t.Fatalf("packet.Kind = %v, want %v", packets[0].Kind, MediaPacketADPCM)
	}
	if packets[0].HasTimestamp {
		t.Fatalf("packet.HasTimestamp = true, want false")
	}
	if !bytes.Equal(packets[0].Data, payload) {
		t.Fatalf("packet.Data = %v, want %v", packets[0].Data, payload)
	}
}
