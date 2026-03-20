package av1

import (
	"testing"
)

// testSeqHdr is a synthetic AV1 Sequence Header OBU for 3840×2160, Profile 0,
// Level 5.1 (idx 13), Main tier, 8-bit, 4:2:0 chroma.
//
// Constructed bit-by-bit from the AV1 spec (Section 5.5):
//
//	OBU header (0x0A): type=1 (seq_header), no extension, has_size=1
//	LEB128 size: 11 bytes
//	Payload bits:
//	  seq_profile=0, still_picture=0, reduced_still=0
//	  timing_info_present=0, initial_display_delay_present=0
//	  operating_points_cnt=1, op_idc=0, level=13, tier=0
//	  frame_width_bits=12, frame_height_bits=12
//	  max_width=3840, max_height=2160
//	  various feature flags, then color_config (8-bit, 4:2:0)
var testSeqHdr = []byte{
	0x0A,                                                       // OBU header: type=1, has_size=1
	0x0B,                                                       // LEB128 payload size = 11
	0x00, 0x00, 0x00, 0x6A, 0xEF, 0xBF, 0xE1, 0xBD, 0xC2, 0x79, 0x00, // payload
}

// testKeyframePayload is [SequenceHeader + Frame(KEY_FRAME)].
var testKeyframePayload = func() []byte {
	// Frame OBU: type=6 (OBU_FRAME), no extension, has_size=1
	// Payload: show_existing_frame=0, frame_type=0 (KEY_FRAME)
	frameOBU := []byte{0x32, 0x01, 0x00} // header=0x32, size=1, payload=0b00000000
	payload := make([]byte, len(testSeqHdr)+len(frameOBU))
	copy(payload, testSeqHdr)
	copy(payload[len(testSeqHdr):], frameOBU)
	return payload
}()

// testInterframePayload is a single Frame OBU with frame_type=INTER_FRAME.
var testInterframePayload = []byte{
	0x32, 0x01, 0x20, // header=0x32, size=1, payload: show_existing=0, frame_type=1 (inter)
}

// --- LEB128 ---

func TestReadLEB128(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		value uint32
		n     int
	}{
		{"zero", []byte{0x00}, 0, 1},
		{"one", []byte{0x01}, 1, 1},
		{"127", []byte{0x7F}, 127, 1},
		{"128", []byte{0x80, 0x01}, 128, 2},
		{"300", []byte{0xAC, 0x02}, 300, 2},
		{"16384", []byte{0x80, 0x80, 0x01}, 16384, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, n := ReadLEB128(tt.input)
			if v != tt.value || n != tt.n {
				t.Errorf("ReadLEB128(%x) = (%d, %d), want (%d, %d)", tt.input, v, n, tt.value, tt.n)
			}
		})
	}
}

func TestWriteLEB128(t *testing.T) {
	tests := []struct {
		value uint32
		want  []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7F}},
		{128, []byte{0x80, 0x01}},
		{300, []byte{0xAC, 0x02}},
	}

	for _, tt := range tests {
		b := WriteLEB128(tt.value)
		if len(b) != len(tt.want) {
			t.Errorf("WriteLEB128(%d) = %x, want %x", tt.value, b, tt.want)
			continue
		}
		for i := range b {
			if b[i] != tt.want[i] {
				t.Errorf("WriteLEB128(%d) = %x, want %x", tt.value, b, tt.want)
				break
			}
		}
	}
}

func TestLEB128Roundtrip(t *testing.T) {
	for _, v := range []uint32{0, 1, 127, 128, 255, 256, 16383, 16384, 1000000} {
		b := WriteLEB128(v)
		got, _ := ReadLEB128(b)
		if got != v {
			t.Errorf("LEB128 roundtrip failed for %d: got %d", v, got)
		}
	}
}

// --- OBU helpers ---

func TestOBUType(t *testing.T) {
	tests := []struct {
		header byte
		want   byte
	}{
		{0x0A, OBUTypeSequenceHeader}, // 0000_1010 → type=1
		{0x12, OBUTypeTemporalDelimiter},
		{0x32, OBUTypeFrame},
		{0x1A, OBUTypeFrameHeader},
		{0x7A, OBUTypePadding},
	}
	for _, tt := range tests {
		got := OBUType(tt.header)
		if got != tt.want {
			t.Errorf("OBUType(0x%02X) = %d, want %d", tt.header, got, tt.want)
		}
	}
}

func TestOBUHasSize(t *testing.T) {
	if !OBUHasSize(0x0A) {
		t.Error("OBUHasSize(0x0A) should be true")
	}
	if OBUHasSize(0x08) {
		t.Error("OBUHasSize(0x08) should be false")
	}
}

func TestOBUHasExtension(t *testing.T) {
	if OBUHasExtension(0x0A) {
		t.Error("OBUHasExtension(0x0A) should be false")
	}
	if !OBUHasExtension(0x0E) {
		t.Error("OBUHasExtension(0x0E) should be true")
	}
}

func TestOBUHeaderSize(t *testing.T) {
	if OBUHeaderSize(0x0A) != 1 {
		t.Error("OBUHeaderSize without extension should be 1")
	}
	if OBUHeaderSize(0x0E) != 2 {
		t.Error("OBUHeaderSize with extension should be 2")
	}
}

// --- ParseOBUs ---

func TestParseOBUs(t *testing.T) {
	var types []byte
	ParseOBUs(testKeyframePayload, func(obuType byte, obu []byte) bool {
		types = append(types, obuType)
		return true
	})

	if len(types) != 2 {
		t.Fatalf("ParseOBUs found %d OBUs, want 2", len(types))
	}
	if types[0] != OBUTypeSequenceHeader {
		t.Errorf("OBU[0] type = %d, want %d (SequenceHeader)", types[0], OBUTypeSequenceHeader)
	}
	if types[1] != OBUTypeFrame {
		t.Errorf("OBU[1] type = %d, want %d (Frame)", types[1], OBUTypeFrame)
	}
}

func TestParseOBUsEarlyStop(t *testing.T) {
	count := 0
	ParseOBUs(testKeyframePayload, func(obuType byte, obu []byte) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Errorf("ParseOBUs with early stop: called %d times, want 1", count)
	}
}

// --- IsKeyframe ---

func TestIsKeyframe(t *testing.T) {
	if !IsKeyframe(testKeyframePayload) {
		t.Error("IsKeyframe should return true for keyframe payload")
	}

	if IsKeyframe(testInterframePayload) {
		t.Error("IsKeyframe should return false for inter-frame payload")
	}
}

func TestIsKeyframeEmpty(t *testing.T) {
	if IsKeyframe(nil) {
		t.Error("IsKeyframe(nil) should return false")
	}
	if IsKeyframe([]byte{}) {
		t.Error("IsKeyframe(empty) should return false")
	}
}

// --- SequenceHeader ---

func TestSequenceHeader(t *testing.T) {
	seqHdr := SequenceHeader(testKeyframePayload)
	if seqHdr == nil {
		t.Fatal("SequenceHeader returned nil")
	}
	if len(seqHdr) != len(testSeqHdr) {
		t.Fatalf("SequenceHeader length = %d, want %d", len(seqHdr), len(testSeqHdr))
	}
	for i := range seqHdr {
		if seqHdr[i] != testSeqHdr[i] {
			t.Errorf("SequenceHeader[%d] = 0x%02X, want 0x%02X", i, seqHdr[i], testSeqHdr[i])
		}
	}
}

func TestSequenceHeaderNotFound(t *testing.T) {
	if SequenceHeader(testInterframePayload) != nil {
		t.Error("SequenceHeader should return nil for payload without seq header")
	}
}

// --- ParseSequenceHeaderInfo ---

func TestParseSequenceHeaderInfo(t *testing.T) {
	info := ParseSequenceHeaderInfo(testSeqHdr)
	if info == nil {
		t.Fatal("ParseSequenceHeaderInfo returned nil")
	}

	if info.Profile != 0 {
		t.Errorf("Profile = %d, want 0", info.Profile)
	}
	if info.Level != 13 {
		t.Errorf("Level = %d, want 13 (5.1)", info.Level)
	}
	if info.Tier != 0 {
		t.Errorf("Tier = %d, want 0 (Main)", info.Tier)
	}
	if info.BitDepth != 8 {
		t.Errorf("BitDepth = %d, want 8", info.BitDepth)
	}
	if info.Width != 3840 {
		t.Errorf("Width = %d, want 3840", info.Width)
	}
	if info.Height != 2160 {
		t.Errorf("Height = %d, want 2160", info.Height)
	}
	if info.Monochrome {
		t.Error("Monochrome should be false")
	}
	if info.ChromaSubsamplingX != 1 || info.ChromaSubsamplingY != 1 {
		t.Errorf("ChromaSubsampling = (%d,%d), want (1,1)", info.ChromaSubsamplingX, info.ChromaSubsamplingY)
	}
}

func TestParseSequenceHeaderInfoNil(t *testing.T) {
	if ParseSequenceHeaderInfo(nil) != nil {
		t.Error("ParseSequenceHeaderInfo(nil) should return nil")
	}
	if ParseSequenceHeaderInfo([]byte{0x0A}) != nil {
		t.Error("ParseSequenceHeaderInfo(too short) should return nil")
	}
}

// --- DecodeSequenceHeader ---

func TestDecodeSequenceHeader(t *testing.T) {
	w, h := DecodeSequenceHeader(testSeqHdr)
	if w != 3840 || h != 2160 {
		t.Errorf("DecodeSequenceHeader = (%d, %d), want (3840, 2160)", w, h)
	}
}

func TestDecodeSequenceHeaderInvalid(t *testing.T) {
	w, h := DecodeSequenceHeader(nil)
	if w != 0 || h != 0 {
		t.Errorf("DecodeSequenceHeader(nil) = (%d, %d), want (0, 0)", w, h)
	}
}

// --- MimeCodecString ---

func TestMimeCodecString(t *testing.T) {
	mime := MimeCodecString(testSeqHdr)
	if mime != "av01.0.13M.08" {
		t.Errorf("MimeCodecString = %q, want %q", mime, "av01.0.13M.08")
	}
}

func TestMimeCodecStringNil(t *testing.T) {
	if MimeCodecString(nil) != "" {
		t.Error("MimeCodecString(nil) should return empty")
	}
}

// --- EncodeConfig ---

func TestEncodeConfig(t *testing.T) {
	conf := EncodeConfig(testSeqHdr)

	// 4-byte av1C header + sequence header OBU as configOBUs
	if len(conf) != 4+len(testSeqHdr) {
		t.Fatalf("EncodeConfig length = %d, want %d", len(conf), 4+len(testSeqHdr))
	}

	// Byte 0: marker=1, version=1 → 0x81
	if conf[0] != 0x81 {
		t.Errorf("conf[0] = 0x%02X, want 0x81 (marker+version)", conf[0])
	}

	// Byte 1: profile(3) | level(5)
	wantProfile := byte(0)
	wantLevel := byte(13)
	if conf[1] != (wantProfile<<5)|wantLevel {
		t.Errorf("conf[1] = 0x%02X, want 0x%02X (profile=%d, level=%d)",
			conf[1], (wantProfile<<5)|wantLevel, wantProfile, wantLevel)
	}

	// Byte 2: tier(1)|highBitdepth(1)|twelveBit(1)|monochrome(1)|chromaX(1)|chromaY(1)|chromaPos(2)
	// tier=0, highBitdepth=0, twelveBit=0, monochrome=0, chromaX=1, chromaY=1, pos=0
	wantByte2 := byte(0<<7 | 0<<6 | 0<<5 | 0<<4 | 1<<3 | 1<<2 | 0)
	if conf[2] != wantByte2 {
		t.Errorf("conf[2] = 0x%02X, want 0x%02X", conf[2], wantByte2)
	}

	// Byte 3: reserved=0
	if conf[3] != 0x00 {
		t.Errorf("conf[3] = 0x%02X, want 0x00", conf[3])
	}

	// Remaining bytes should be the sequence header OBU
	for i, b := range testSeqHdr {
		if conf[4+i] != b {
			t.Errorf("conf[%d] = 0x%02X, want 0x%02X (seqHdr[%d])", 4+i, conf[4+i], b, i)
		}
	}
}

func TestEncodeConfigDefaults(t *testing.T) {
	conf := EncodeConfig(nil)
	if len(conf) != 4 {
		t.Fatalf("EncodeConfig(nil) length = %d, want 4", len(conf))
	}
	if conf[0] != 0x81 {
		t.Errorf("default conf[0] = 0x%02X, want 0x81", conf[0])
	}
	// Default level = 8 (4.0)
	if conf[1]&0x1F != 8 {
		t.Errorf("default level = %d, want 8", conf[1]&0x1F)
	}
}

// --- minLevelForResolution ---

func TestMinLevelForResolution(t *testing.T) {
	tests := []struct {
		w, h uint16
		want byte
	}{
		{640, 480, 0},     // SD → any level
		{1920, 1080, 9},   // 1080p → Level 4.1
		{2560, 1440, 13},  // 1440p → Level 5.1
		{3840, 2160, 13},  // 4K → Level 5.1
		{7680, 4320, 17},  // 8K → Level 6.1
	}
	for _, tt := range tests {
		got := minLevelForResolution(tt.w, tt.h)
		if got != tt.want {
			t.Errorf("minLevelForResolution(%d, %d) = %d, want %d", tt.w, tt.h, got, tt.want)
		}
	}
}
