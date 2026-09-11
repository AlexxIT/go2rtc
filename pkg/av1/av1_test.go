package av1

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

type bitWriter struct {
	data []byte
	n    int
}

func (w *bitWriter) put(val uint32, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.data = append(w.data, 0)
		}
		w.data[w.n/8] |= byte((val>>uint(i))&1) << uint(7-w.n%8)
		w.n++
	}
}

func (w *bitWriter) flag(b bool) {
	if b {
		w.put(1, 1)
	} else {
		w.put(0, 1)
	}
}

func wrapOBU(obuType byte, payload []byte) []byte {
	obu := []byte{obuType<<3 | 0x02} // has_size_field=1
	obu = append(obu, WriteLEB128(uint32(len(payload)))...)
	return append(obu, payload...)
}

// seqHdr builds a Sequence Header OBU bit by bit, following
// sequence_header_obu() from AV1 spec Section 5.5.1. The optional fields have
// their own switches so tests can cover what real encoders actually emit.
type seqHdr struct {
	profile       byte
	level         byte
	tier          byte
	width, height uint32
	opCount       uint32

	timingInfo   bool // timing_info_present_flag + decoder_model_info
	displayDelay bool // initial_display_delay_present_flag
	reducedStill bool // reduced_still_picture_header

	highBitdepth bool
	twelveBit    bool
	monochrome   bool
	subX, subY   byte // only written for profile 2 at 12-bit
}

func (s seqHdr) build() []byte {
	if s.opCount == 0 {
		s.opCount = 1
	}
	if s.width == 0 {
		s.width, s.height = 1920, 1080
	}

	w := &bitWriter{}
	w.put(uint32(s.profile), 3)
	w.flag(false) // still_picture
	w.flag(s.reducedStill)

	if s.reducedStill {
		w.put(uint32(s.level), 5)
	} else {
		w.flag(s.timingInfo)
		if s.timingInfo {
			w.put(0, 32)  // num_units_in_display_tick
			w.put(0, 32)  // time_scale
			w.flag(false) // equal_picture_interval
			w.flag(true)  // decoder_model_info_present_flag
			w.put(15, 5)  // buffer_delay_length_minus_1, 16 bits per delay
			w.put(0, 32)  // num_units_in_decoding_tick
			w.put(0, 5)   // buffer_removal_time_length_minus_1
			w.put(0, 5)   // frame_presentation_time_length_minus_1
		}

		w.flag(s.displayDelay)

		w.put(s.opCount-1, 5)
		for i := uint32(0); i < s.opCount; i++ {
			w.put(0, 12) // operating_point_idc
			w.put(uint32(s.level), 5)
			if s.level > 7 {
				w.put(uint32(s.tier), 1)
			}
			if s.timingInfo {
				w.flag(true) // decoder_model_present_for_this_op
				w.put(0, 16) // decoder_buffer_delay
				w.put(0, 16) // encoder_buffer_delay
				w.flag(false)
			}
			if s.displayDelay {
				w.flag(true) // initial_display_delay_present_for_this_op
				w.put(0, 4)  // initial_display_delay_minus_1
			}
		}
	}

	w.put(15, 4) // frame_width_bits_minus_1
	w.put(15, 4) // frame_height_bits_minus_1
	w.put(s.width-1, 16)
	w.put(s.height-1, 16)

	if !s.reducedStill {
		w.flag(false) // frame_id_numbers_present_flag
	}

	w.flag(false) // use_128x128_superblock
	w.flag(false) // enable_filter_intra
	w.flag(false) // enable_intra_edge_filter

	if !s.reducedStill {
		w.flag(false) // enable_interintra_compound
		w.flag(false) // enable_masked_compound
		w.flag(false) // enable_warped_motion
		w.flag(false) // enable_dual_filter
		w.flag(false) // enable_order_hint
		w.flag(true)  // seq_choose_screen_content_tools
		w.flag(true)  // seq_choose_integer_mv
	}

	w.flag(false) // enable_superres
	w.flag(false) // enable_cdef
	w.flag(false) // enable_restoration

	// color_config()
	w.flag(s.highBitdepth)
	if s.profile == 2 && s.highBitdepth {
		w.flag(s.twelveBit)
	}
	if s.profile != 1 {
		w.flag(s.monochrome)
	}
	w.flag(false) // color_description_present_flag
	w.put(0, 1)   // color_range

	if !s.monochrome {
		if s.profile == 2 && s.highBitdepth && s.twelveBit {
			w.put(uint32(s.subX), 1)
			if s.subX == 1 {
				w.put(uint32(s.subY), 1)
			}
		}
		subX, subY := s.subX, s.subY
		switch {
		case s.profile == 0:
			subX, subY = 1, 1
		case s.profile == 1:
			subX, subY = 0, 0
		case !(s.highBitdepth && s.twelveBit):
			subX, subY = 1, 0
		}
		if subX == 1 && subY == 1 {
			w.put(0, 2) // chroma_sample_position
		}
		w.flag(false) // separate_uv_delta_q
	}

	w.flag(false) // film_grain_params_present

	return wrapOBU(OBUTypeSequenceHeader, w.data)
}

var testSeqHdr = seqHdr{level: 13, width: 3840, height: 2160}.build()

func keyframeOBU() []byte   { return wrapOBU(OBUTypeFrame, []byte{0x00}) }
func interframeOBU() []byte { return wrapOBU(OBUTypeFrame, []byte{0x20}) }

var testKeyframePayload = append(append([]byte{}, testSeqHdr...), keyframeOBU()...)

// --- LEB128 ---

func TestReadLEB128(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		value uint32
		n     int
	}{
		{"zero", []byte{0x00}, 0, 1},
		{"one byte", []byte{0x7F}, 127, 1},
		{"two bytes", []byte{0x80, 0x01}, 128, 2},
		{"three bytes", []byte{0xE5, 0x8E, 0x26}, 624485, 3},
		{"empty", nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, n := ReadLEB128(tt.input)
			require.Equal(t, tt.value, value, "ReadLEB128(%v) value", tt.input)
			require.Equal(t, tt.n, n, "ReadLEB128(%v) bytes consumed", tt.input)
		})
	}
}

func TestLEB128Roundtrip(t *testing.T) {
	for _, v := range []uint32{0, 1, 127, 128, 255, 256, 16383, 16384, 624485, 1 << 28} {
		got, n := ReadLEB128(WriteLEB128(v))
		require.Equal(t, v, got, "LEB128 round trip for %d", v)
		require.NotZero(t, n, "LEB128 round trip for %d consumed nothing", v)
	}
}

// --- OBU helpers ---

func TestOBUHeader(t *testing.T) {
	tests := []struct {
		header  byte
		obuType byte
		hasSize bool
		hasExt  bool
		hdrSize int
	}{
		{0x0A, OBUTypeSequenceHeader, true, false, 1},
		{0x08, OBUTypeSequenceHeader, false, false, 1},
		{0x0E, OBUTypeSequenceHeader, true, true, 2},
		{0x12, OBUTypeTemporalDelimiter, true, false, 1},
		{0x1A, OBUTypeFrameHeader, true, false, 1},
		{0x32, OBUTypeFrame, true, false, 1},
	}
	for _, tt := range tests {
		require.Equal(t, tt.obuType, OBUType(tt.header), "OBUType(0x%02X)", tt.header)
		require.Equal(t, tt.hasSize, OBUHasSize(tt.header), "OBUHasSize(0x%02X)", tt.header)
		require.Equal(t, tt.hasExt, OBUHasExtension(tt.header), "OBUHasExtension(0x%02X)", tt.header)
		require.Equal(t, tt.hdrSize, OBUHeaderSize(tt.header), "OBUHeaderSize(0x%02X)", tt.header)
	}
}

// --- ParseOBUs ---

func TestParseOBUs(t *testing.T) {
	var types []byte
	ParseOBUs(testKeyframePayload, func(obuType byte, obu []byte) bool {
		types = append(types, obuType)
		return true
	})

	want := []byte{OBUTypeSequenceHeader, OBUTypeFrame}
	require.Equal(t, want, types, "ParseOBUs found %v, want %v", types, want)
}

func TestParseOBUsEarlyStop(t *testing.T) {
	count := 0
	ParseOBUs(testKeyframePayload, func(obuType byte, obu []byte) bool {
		count++
		return false
	})
	require.Equal(t, 1, count, "ParseOBUs with early stop: called %d times, want 1", count)
}

func TestParseOBUsTruncated(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int // how many OBUs must be reported
	}{
		// size field promises more data than the buffer holds
		{"far past the end", []byte{0x0A, 0x40, 0x00, 0x00}, 0},
		// one byte short, the boundary a loose bounds check would let through
		{"over-runs by one", []byte{0x32, 0x03, 0x00, 0x00}, 0},
		// exactly fits, must be reported
		{"exact fit", []byte{0x32, 0x02, 0x00, 0x00}, 1},
		// size field itself is not a valid LEB128
		{"unterminated size", []byte{0x32, 0x80, 0x80, 0x80, 0x80, 0x80}, 0},
		// 5th byte carries bits above 32
		{"size over 32 bits", []byte{0x32, 0x80, 0x80, 0x80, 0x80, 0x10}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n int
			ParseOBUs(tt.data, func(obuType byte, obu []byte) bool {
				n++
				return true
			})
			require.Equal(t, tt.want, n, "reported %d OBUs, want %d", n, tt.want)
		})
	}
}

// --- IsKeyframe ---

func TestIsKeyframe(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{"keyframe", testKeyframePayload, true},
		{"interframe", interframeOBU(), false},
		{"frame only, no seq header", keyframeOBU(), true},
		{
			"temporal delimiter first",
			append(wrapOBU(OBUTypeTemporalDelimiter, nil), testKeyframePayload...),
			true,
		},
		{
			// the last OBU of a temporal unit may omit the size field
			"no size field",
			[]byte{OBUTypeFrame<<3 | 0x00, 0x00},
			true,
		},
		{
			"frame header plus tile group",
			append(wrapOBU(OBUTypeFrameHeader, []byte{0x00}), wrapOBU(OBUTypeTileGroup, []byte{0x00})...),
			true,
		},
		{
			// show_existing_frame carries no frame_type, must not count as key
			"show existing frame",
			wrapOBU(OBUTypeFrame, []byte{0x80}),
			false,
		},
		{
			// a whole sample written by ffmpeg/libsvtav1, a frame header that
			// only shows an already decoded frame
			"show existing frame, ffmpeg sample",
			[]byte{0x1A, 0x01, 0x88},
			false,
		},
		{
			// reduced_still_picture_header implies KEY_FRAME for every frame
			"reduced still picture",
			append(seqHdr{reducedStill: true, level: 8}.build(), interframeOBU()...),
			true,
		},
		{
			// a sequence header alone is not a frame
			"sequence header only",
			testSeqHdr,
			false,
		},
		{
			// INTRA_ONLY is not a keyframe, and it is the only frame type that
			// tells a correct frame_type read from a shifted one
			"intra only frame",
			wrapOBU(OBUTypeFrame, []byte{0x40}),
			false,
		},
		{
			"switch frame",
			wrapOBU(OBUTypeFrame, []byte{0x60}),
			false,
		},
		{
			// the first frame header of a unit decides, not the last
			"keyframe followed by inter frame",
			append(wrapOBU(OBUTypeFrame, []byte{0x00}), wrapOBU(OBUTypeFrame, []byte{0x20})...),
			true,
		},
		{
			"inter frame followed by keyframe",
			append(wrapOBU(OBUTypeFrame, []byte{0x20}), wrapOBU(OBUTypeFrame, []byte{0x00})...),
			false,
		},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsKeyframe(tt.payload))
		})
	}
}

// --- SequenceHeader ---

func TestSequenceHeader(t *testing.T) {
	require.Equal(t, testSeqHdr, SequenceHeader(testKeyframePayload))
	require.Nil(t, SequenceHeader(interframeOBU()), "an inter frame carries no sequence header")
}

// --- ParseSequenceHeaderInfo ---

func TestParseSequenceHeaderInfo(t *testing.T) {
	tests := []struct {
		name string
		hdr  seqHdr
		want SequenceHeaderInfo
		mime string
	}{
		{
			name: "1080p main",
			hdr:  seqHdr{level: 8},
			want: SequenceHeaderInfo{Level: 8, BitDepth: 8, Width: 1920, Height: 1080,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.08M.08",
		},
		{
			name: "4k high tier",
			hdr:  seqHdr{level: 13, tier: 1, width: 3840, height: 2160},
			want: SequenceHeaderInfo{Level: 13, Tier: 1, BitDepth: 8, Width: 3840, Height: 2160,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.13H.08",
		},
		{
			// the two per operating point flags below are what ffmpeg and
			// several cameras emit, and skipping them shifts everything after
			name: "initial_display_delay",
			hdr:  seqHdr{level: 8, displayDelay: true},
			want: SequenceHeaderInfo{Level: 8, BitDepth: 8, Width: 1920, Height: 1080,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.08M.08",
		},
		{
			name: "decoder_model_info",
			hdr:  seqHdr{level: 8, timingInfo: true},
			want: SequenceHeaderInfo{Level: 8, BitDepth: 8, Width: 1920, Height: 1080,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.08M.08",
		},
		{
			name: "both, three operating points",
			hdr:  seqHdr{level: 13, width: 3840, height: 2160, opCount: 3, timingInfo: true, displayDelay: true},
			want: SequenceHeaderInfo{Level: 13, BitDepth: 8, Width: 3840, Height: 2160,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.13M.08",
		},
		{
			name: "reduced still picture",
			hdr:  seqHdr{level: 8, reducedStill: true, width: 1280, height: 720},
			want: SequenceHeaderInfo{Level: 8, BitDepth: 8, Width: 1280, Height: 720,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.08M.08",
		},
		{
			name: "10 bit",
			hdr:  seqHdr{level: 13, width: 3840, height: 2160, highBitdepth: true},
			want: SequenceHeaderInfo{Level: 13, BitDepth: 10, Width: 3840, Height: 2160,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1, highBitdepth: 1},
			mime: "av01.0.13M.10",
		},
		{
			name: "profile 2, 12 bit 4:2:0",
			hdr:  seqHdr{profile: 2, level: 8, highBitdepth: true, twelveBit: true, subX: 1, subY: 1},
			want: SequenceHeaderInfo{Profile: 2, Level: 8, BitDepth: 12, Width: 1920, Height: 1080,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1, highBitdepth: 1, twelveBit: 1},
			mime: "av01.2.08M.12",
		},
		{
			name: "profile 2, 12 bit 4:4:4",
			hdr:  seqHdr{profile: 2, level: 8, highBitdepth: true, twelveBit: true},
			want: SequenceHeaderInfo{Profile: 2, Level: 8, BitDepth: 12, Width: 1920, Height: 1080,
				highBitdepth: 1, twelveBit: 1},
			mime: "av01.2.08M.12",
		},
		{
			// spec keeps subsampling at 1,1 for monochrome
			name: "monochrome",
			hdr:  seqHdr{level: 8, monochrome: true},
			want: SequenceHeaderInfo{Level: 8, BitDepth: 8, Width: 1920, Height: 1080, Monochrome: true,
				ChromaSubsamplingX: 1, ChromaSubsamplingY: 1},
			mime: "av01.0.08M.08",
		},
		{
			name: "profile 1 is 4:4:4",
			hdr:  seqHdr{profile: 1, level: 8},
			want: SequenceHeaderInfo{Profile: 1, Level: 8, BitDepth: 8, Width: 1920, Height: 1080},
			mime: "av01.1.08M.08",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.hdr.build()

			info := ParseSequenceHeaderInfo(raw)
			require.NotNil(t, info, "ParseSequenceHeaderInfo returned nil")
			require.Equal(t, tt.want, *info)

			require.Equal(t, tt.mime, MimeCodecString(raw))

			// the av1C packing has a field per parsed value, check them all
			var mono byte
			if tt.want.Monochrome {
				mono = 1
			}
			conf := EncodeConfig(raw)
			wantSeq := tt.want.Profile<<5 | tt.want.Level&0x1F
			wantColor := tt.want.Tier<<7 | tt.want.highBitdepth<<6 | tt.want.twelveBit<<5 | mono<<4 |
				tt.want.ChromaSubsamplingX<<3 | tt.want.ChromaSubsamplingY<<2 | tt.want.ChromaSamplePos&0x03
			require.Equal(t, wantSeq, conf[1], "av1C seq_profile/seq_level_idx byte")
			require.Equal(t, wantColor, conf[2], "av1C colour byte")

			w, h := DecodeSequenceHeader(raw)
			require.Equal(t, tt.want.Width, w, "width")
			require.Equal(t, tt.want.Height, h, "height")
		})
	}
}

func TestParseSequenceHeaderInfoInvalid(t *testing.T) {
	require.Nil(t, ParseSequenceHeaderInfo(nil), "nil input should not parse")
	require.Equal(t, "", MimeCodecString(nil), "MimeCodecString(nil) should be empty")

	// reserved profile
	require.Nil(t, ParseSequenceHeaderInfo(seqHdr{profile: 5, level: 8}.build()), "a reserved profile must not parse")

	// every truncation must be rejected rather than parsed into garbage
	full := testSeqHdr
	for i := 0; i < len(full); i++ {
		if info := ParseSequenceHeaderInfo(full[:i]); info != nil {
			t.Errorf("truncated to %d/%d bytes parsed as %+v", i, len(full), *info)
		}
	}

	// size field is honest but the payload ends mid header
	payload := full[2:]
	for i := 1; i < len(payload); i++ {
		if info := ParseSequenceHeaderInfo(wrapOBU(OBUTypeSequenceHeader, payload[:i])); info != nil {
			t.Errorf("payload cut to %d/%d bytes parsed as %+v", i, len(payload), *info)
		}
	}
}

// --- EncodeConfig ---

func TestEncodeConfig(t *testing.T) {
	conf := EncodeConfig(testSeqHdr)

	require.Equal(t, 4+len(testSeqHdr), len(conf), "av1C is a 4 byte record plus the configOBUs")
	require.Equal(t, byte(0x81), conf[0], "marker=1, version=1")
	// profile(3)=0 | seq_level_idx_0(5)=13
	require.Equal(t, byte(13), conf[1], "conf[1] = 0x%02X, want 0x0D (profile=0, level=13)", conf[1])
	// tier|high_bitdepth|twelve_bit|monochrome|chroma_x|chroma_y|chroma_pos(2)
	require.Equal(t, byte(1<<3|1<<2), conf[2], "conf[2] = 0x%02X, want 0x0C (4:2:0, 8-bit)", conf[2])
	require.Equal(t, byte(0x00), conf[3], "conf[3] = 0x%02X, want 0x00", conf[3])
	require.Equal(t, testSeqHdr, conf[4:], "configOBUs should be the sequence header OBU")
}

func TestEncodeConfigDefaults(t *testing.T) {
	conf := EncodeConfig(nil)
	require.Equal(t, 4, len(conf), "av1C without configOBUs is 4 bytes")
	require.Equal(t, byte(0x81), conf[0], "marker=1, version=1")
	require.Equal(t, byte(8), conf[1]&0x1F, "default level is 4.0")
	require.Equal(t, byte(1<<3|1<<2), conf[2], "conf[2] = 0x%02X, want 0x0C (4:2:0)", conf[2])
}

// --- FmtpLine ---

func TestFmtpLine(t *testing.T) {
	require.Equal(t, testSeqHdr, GetSequenceHeader(EncodeFmtpLine(testSeqHdr)), "fmtp line round trip")
	require.Nil(t, GetSequenceHeader(""), "an empty fmtp line has no sequence header")
	for _, fmtp := range []string{
		"packetization-mode=1",
		fmtpSeqHeader,
		fmtpSeqHeader + ";profile=0",
		fmtpSeqHeader + "not base64!;",
	} {
		require.Nil(t, GetSequenceHeader(fmtp), "GetSequenceHeader(%q)", fmtp)
	}

	// no trailing semicolon
	require.Equal(t, testSeqHdr, GetSequenceHeader(EncodeFmtpLine(testSeqHdr)+";x=1"), "trailing fmtp params")
}

// --- real world fixtures ---

// av1C records taken from files written by ffmpeg/libsvtav1. Re-encoding them
// must reproduce the original bytes, which checks the parser and the encoder
// against a reference implementation rather than against our own builder.
func TestEncodeConfigAgainstFFmpeg(t *testing.T) {
	tests := []struct {
		name string
		av1c string
		mime string
		w, h uint16
	}{
		{"640x480 8-bit 4:2:0", "81040c000a0b00000024c4ffdf00be0010", "av01.0.04M.08", 640, 480},
		{"1920x1080 8-bit 4:2:0", "81080c000a0b00000042abbfc3700be001", "av01.0.08M.08", 1920, 1080},
		{"1280x720 8-bit 4:2:0", "81050c000a0b0000002d4cffb3c02f8004", "av01.0.05M.08", 1280, 720},
		{"3840x2160 10-bit 4:2:0", "810c4c000a0c00000062efbfe1bc02f84040", "av01.0.12M.10", 3840, 2160},
		// color_description_present_flag = 1, which no other fixture exercises
		{"1920x1080 BT.709 tagged", "81080c000a0e00000042abbfc3700be040808041", "av01.0.08M.08", 1920, 1080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := hex.DecodeString(tt.av1c)
			require.NoError(t, err, err)
			seqHdr := want[4:] // configOBUs

			require.Equal(t, want, EncodeConfig(seqHdr), "av1C record")
			require.Equal(t, tt.mime, MimeCodecString(seqHdr))
			if w, h := DecodeSequenceHeader(seqHdr); w != tt.w || h != tt.h {
				t.Errorf("DecodeSequenceHeader() = (%d, %d), want (%d, %d)", w, h, tt.w, tt.h)
			}
		})
	}
}

// A sequence header without a size field runs to the end of the temporal unit.
// Returning it would put the whole keyframe into av1C and into the fmtp line.
func TestSequenceHeaderWithoutSizeField(t *testing.T) {
	hdr := seqHdr{level: 8}.build()
	sizeless := append([]byte{hdr[0] &^ 0x02}, hdr[2:]...)
	unit := append(sizeless, wrapOBU(OBUTypeFrame, make([]byte, 4096))...)

	require.Nil(t, SequenceHeader(unit), "a sizeless sequence header must be refused")
}

// LEB128 that is truncated or wider than 32 bits must be rejected, not read as 0.
func TestReadLEB128Invalid(t *testing.T) {
	for _, b := range [][]byte{
		{0x80},                               // no terminating byte
		{0x80, 0x80},                         // still continuing
		{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}, // 2^35
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // never terminates
	} {
		if v, n := ReadLEB128(b); n != 0 {
			t.Errorf("ReadLEB128(% x) = (%d, %d), want n = 0", b, v, n)
		}
	}
}

// uvlc() counts zeros up to the terminating one bit and only then checks the
// range. Stopping at 32 leaves the reader a few bits behind for the rest of
// the header, which yields a plausible but wrong resolution.
func TestReadUVLC(t *testing.T) {
	tests := []struct {
		name string
		bits []uint32 // value, width pairs written in order
		want uint32
	}{
		{"zero", []uint32{1, 1}, 0},
		{"one", []uint32{0, 1, 1, 1, 0, 1}, 1},
		{"two", []uint32{0, 1, 1, 1, 1, 1}, 2},
		{"255", []uint32{0, 8, 1, 1, 0, 8}, 255},  // (1<<8 - 1) + 0
		{"300", []uint32{0, 8, 1, 1, 45, 8}, 300}, // (1<<8 - 1) + 45
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bitWriter{}
			for i := 0; i < len(tt.bits); i += 2 {
				w.put(tt.bits[i], int(tt.bits[i+1]))
			}
			r := &bitReader{data: w.data}
			require.Equal(t, tt.want, r.readUVLC())
		})
	}

	// 33 leading zeros is out of range, and the terminator still has to be
	// consumed so the caller can reject the header instead of misreading it
	w := &bitWriter{}
	w.put(0, 33)
	w.put(1, 1)
	w.put(0x2A, 8)
	r := &bitReader{data: w.data}
	require.Equal(t, uint32(0xFFFFFFFF), r.readUVLC(), "32 leading zeros")
	require.Equal(t, uint32(0x2A), r.readBits(8), "reader is out of sync after an oversized uvlc")
}

// The guard exists for exactly this value.
func TestClampDimension(t *testing.T) {
	for _, tt := range []struct {
		minusOne uint32
		want     uint16
	}{{0, 1}, {1919, 1920}, {0xFFFE, 0xFFFF}, {0xFFFF, 0xFFFF}, {0x1FFFF, 0xFFFF}} {
		require.Equal(t, tt.want, clampDimension(tt.minusOne), "clampDimension(%d)", tt.minusOne)
	}
}

// A header that ends exactly on its last bit is valid and must be accepted.
func TestBitReaderExactFit(t *testing.T) {
	r := &bitReader{data: []byte{0xFF, 0xFF}}
	require.Equal(t, uint32(0xFFFF), r.readBits(16), "an exact fit must read")
	require.False(t, r.eof, "an exact fit must not set eof")

	r.readBits(1)
	require.True(t, r.eof, "reading past the end must set eof")

	r = &bitReader{data: []byte{0xFF}}
	r.skipBits(8)
	if r.eof {
		t.Error("skipping exactly to the end should not set eof")
	}
	r.skipBits(1)
	require.True(t, r.eof, "skipping past the end should set eof")
}

// --- RTP ---

func TestRTPRoundTrip(t *testing.T) {
	// a temporal unit big enough to need fragmenting at every MTU below
	unit := append(append([]byte{}, testSeqHdr...),
		wrapOBU(OBUTypeFrame, append([]byte{0x00}, bytes.Repeat([]byte{0xA5}, 4000)...))...)

	for _, mtu := range []uint16{1472, 1200, 300, 64, 20} {
		t.Run(fmt.Sprint(mtu), func(t *testing.T) {
			var got [][]byte
			var out *rtp.Packet
			sink := RTPDepay(func(p *rtp.Packet) {
				got = append(got, append([]byte{}, p.Payload...))
				out = p
			})

			var last bool
			var maxPayload int
			pay := RTPPay(mtu, func(p *rtp.Packet) {
				last = p.Marker
				if len(p.Payload) > maxPayload {
					maxPayload = len(p.Payload)
				}
				sink(p)
			})
			pay(&rtp.Packet{
				Header:  rtp.Header{Version: RTPPacketVersionAV1, Timestamp: 90000},
				Payload: unit,
			})

			require.True(t, last, "the last packet has no marker")
			require.Equal(t, 1, len(got), "temporal units")
			require.Equal(t, unit, got[0], "round trip changed the unit: %d bytes in, %d out", len(unit), len(got[0]))
			require.True(t, IsKeyframe(got[0]), "the reassembled unit is no longer a keyframe")

			// RTPPay keys off the version to tell depayed units apart
			require.Equal(t, uint8(RTPPacketVersionAV1), out.Version, "RTPPay keys off the version")
			// AV1 has no B-frames, a composition time offset would desync the muxer
			require.Equal(t, uint16(0), out.ExtensionProfile, "AV1 has no B-frames, so no composition time offset")
			// the payloader has to leave room for the 12 byte RTP header
			if maxPayload+12 > int(mtu) {
				t.Errorf("largest packet is %d+12 bytes, over the %d byte MTU", maxPayload, mtu)
			}
		})
	}
}

// A lost packet splices OBUs from two units together, and pion rewrites
// obu_size to match, so the result still parses as a valid keyframe.
func TestRTPDepayDropsGaps(t *testing.T) {
	unit := append(append([]byte{}, testSeqHdr...),
		wrapOBU(OBUTypeFrame, append([]byte{0x00}, bytes.Repeat([]byte{0xA5}, 4000)...))...)

	var packets []*rtp.Packet
	seq := uint16(1000)
	RTPPay(300, func(p *rtp.Packet) {
		p.SequenceNumber = seq
		seq++
		packets = append(packets, p)
	})(&rtp.Packet{Header: rtp.Header{Version: RTPPacketVersionAV1}, Payload: unit})

	if len(packets) < 4 {
		t.Fatalf("expected a fragmented unit, got %d packets", len(packets))
	}

	// a hole anywhere in the unit must drop it, not emit a shortened one:
	// pion rewrites obu_size on reassembly, so the remains still parse
	for drop := range packets {
		var got [][]byte
		h := RTPDepay(func(p *rtp.Packet) { got = append(got, append([]byte{}, p.Payload...)) })
		for i, p := range packets {
			if i == drop {
				continue
			}
			h(p)
		}

		for _, u := range got {
			if !bytes.Equal(u, unit) {
				t.Errorf("dropping packet %d/%d emitted %d bytes, want %d or nothing",
					drop, len(packets), len(u), len(unit))
			}
		}
	}
}

// TestRTPDepayResyncsOnUnitBoundary - after a hole the depayloader must not
// resume mid unit, or the tail of the broken unit is spliced onto the next one.
func TestRTPDepayResyncsOnUnitBoundary(t *testing.T) {
	unitA := append(append([]byte{}, testSeqHdr...),
		wrapOBU(OBUTypeFrame, append([]byte{0x00}, bytes.Repeat([]byte{0xA5}, 2000)...))...)
	unitB := wrapOBU(OBUTypeFrame, append([]byte{0x30}, bytes.Repeat([]byte{0x5A}, 2000)...))

	var packets []*rtp.Packet
	seq := uint16(1000)
	pay := RTPPay(300, func(p *rtp.Packet) {
		p.SequenceNumber = seq
		seq++
		packets = append(packets, p)
	})
	pay(&rtp.Packet{Header: rtp.Header{Version: RTPPacketVersionAV1}, Payload: unitA})
	pay(&rtp.Packet{Header: rtp.Header{Version: RTPPacketVersionAV1}, Payload: unitB})

	for drop := range packets {
		var got [][]byte
		h := RTPDepay(func(p *rtp.Packet) { got = append(got, append([]byte{}, p.Payload...)) })
		for i, p := range packets {
			if i == drop {
				continue
			}
			h(p)
		}

		for _, u := range got {
			if !bytes.Equal(u, unitA) && !bytes.Equal(u, unitB) {
				t.Errorf("dropping packet %d/%d emitted a %d byte unit that is neither source unit",
					drop, len(packets), len(u))
			}
		}
	}
}

// The depacketizer buffers fragments of an unfinished OBU itself, so a stream
// of continuation fragments that never completes one must not grow without end.
func TestRTPDepayBoundsUnfinishedUnit(t *testing.T) {
	var emitted int
	depay := RTPDepay(func(p *rtp.Packet) { emitted++ })

	seq := uint16(1)
	depay(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq}, Payload: []byte{0x50, 0x30, 0x00}})

	frag := make([]byte, 1200)
	frag[0] = 0xD0 // Z=1 Y=1 W=1, a continuation that never ends

	// well past maxUnitBytes: without a bound this is quadratic and keeps
	// every byte, so it would take tens of seconds instead of well under one
	start := time.Now()
	for i := 0; i < 32000; i++ {
		seq++
		depay(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq}, Payload: frag})
	}
	elapsed := time.Since(start)

	require.Equal(t, 0, emitted, "emitted %d units from fragments that never complete an OBU", emitted)
	if elapsed > 5*time.Second {
		t.Errorf("38 MB of continuation fragments took %v, the buffer is not bounded", elapsed)
	}
}

// A large but legal temporal unit still has to survive the round trip.
func TestRTPRoundTripLargeUnit(t *testing.T) {
	unit := append(append([]byte{}, testSeqHdr...),
		wrapOBU(OBUTypeFrame, append([]byte{0x00}, bytes.Repeat([]byte{0x5A}, 1<<20)...))...)

	var got [][]byte
	sink := RTPDepay(func(p *rtp.Packet) { got = append(got, append([]byte{}, p.Payload...)) })
	RTPPay(1200, sink)(&rtp.Packet{
		Header:  rtp.Header{Version: RTPPacketVersionAV1, Timestamp: 90000},
		Payload: unit,
	})

	if len(got) != 1 || !bytes.Equal(got[0], unit) {
		t.Fatalf("a 1 MB unit did not survive the round trip: %d units back", len(got))
	}
}

// A sequence header is copied into the fmtp line, into the av1C box of every
// init segment and into every RTSP SDP answer, so a padded one must be refused.
func TestSequenceHeaderSizeCap(t *testing.T) {
	for _, n := range []int{16, MaxSequenceHeaderSize - 4, MaxSequenceHeaderSize + 1, 1 << 20} {
		body := make([]byte, n)
		obu := append([]byte{0x0A}, WriteLEB128(uint32(n))...)
		obu = append(obu, body...)

		got := SequenceHeader(obu)
		if n <= MaxSequenceHeaderSize-4 && got == nil {
			t.Errorf("a %d byte sequence header was refused", n)
		}
		if n > MaxSequenceHeaderSize && got != nil {
			t.Errorf("a %d byte sequence header was accepted, %d bytes came back", n, len(got))
		}
	}
}

// int is 32 bits on the 386, arm and mips builds, where a size this large goes
// negative and the slice bounds check is passed with a negative length.
func TestParseOBUsHugeSizeField(t *testing.T) {
	data := []byte{0x0A, 0x80, 0x80, 0x80, 0x80, 0x08} // obu_size = 0x80000000

	var n int
	ParseOBUs(data, func(obuType byte, obu []byte) bool { n++; return true })
	require.Equal(t, 0, n, "reported %d OBUs for a size field of 2 GiB", n)

	IsKeyframe(data)
	SequenceHeader(data)
}

// pion's payloader marks every packet after the first as a continuation, but a
// foreign packetizer may start a fresh OBU in the middle of a temporal unit.
// Resuming there would emit a unit missing everything before the gap.
func TestRTPDepayForeignPacketizerMidUnit(t *testing.T) {
	obu := func(b byte) []byte { return []byte{0x32, 0x02, b, b} }
	// aggregation header: Z=0 (starts an OBU), W=1 element
	pkt := func(seq uint16, marker bool, b byte) *rtp.Packet {
		return &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: seq, Marker: marker},
			Payload: append([]byte{0x10}, obu(b)...),
		}
	}

	var got [][]byte
	h := RTPDepay(func(p *rtp.Packet) { got = append(got, append([]byte{}, p.Payload...)) })

	h(pkt(100, false, 0xAA)) // starts the unit
	// sequence 101 is lost
	h(pkt(102, false, 0xCC)) // fresh OBU, but still mid unit
	h(pkt(103, true, 0xDD))  // ends the unit

	require.Empty(t, got, "a unit missing everything before the gap was emitted")
}
