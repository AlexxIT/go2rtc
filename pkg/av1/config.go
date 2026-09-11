package av1

import (
	"encoding/base64"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Color description values from AV1 spec Section 6.4.2.
const (
	cpBT709    = 1
	tcSRGB     = 13
	mcIdentity = 0
)

// SequenceHeaderInfo holds parsed fields from an AV1 Sequence Header OBU.
// Used for av1C box generation, MIME codec strings, and resolution detection.
type SequenceHeaderInfo struct {
	Profile    byte
	Level      byte // seq_level_idx[0]
	Tier       byte // 0=Main, 1=High
	BitDepth   byte // 8, 10, or 12
	Monochrome bool
	Width      uint16
	Height     uint16

	// Chroma subsampling (for av1C box)
	ChromaSubsamplingX byte
	ChromaSubsamplingY byte
	ChromaSamplePos    byte

	// Raw flags for av1C encoding
	highBitdepth byte
	twelveBit    byte
}

// ParseSequenceHeaderInfo parses a raw Sequence Header OBU (with OBU header and
// optional size field). Returns nil if the OBU is truncated or invalid.
//
// Follows sequence_header_obu() from AV1 spec Section 5.5.1.
func ParseSequenceHeaderInfo(obu []byte) *SequenceHeaderInfo {
	data := obuPayload(obu)
	if data == nil {
		return nil
	}

	r := &bitReader{data: data}
	info := &SequenceHeaderInfo{}

	info.Profile = byte(r.readBits(3))
	if info.Profile > 2 {
		return nil // reserved profile, rest of the header can't be trusted
	}

	r.skipBits(1) // still_picture
	reducedStill := r.readFlag()

	// flags that gate the per operating point fields below
	var decoderModelInfo, initialDisplayDelay bool
	var bufferDelayLength uint32

	if reducedStill {
		info.Level = byte(r.readBits(5))
	} else {
		if r.readFlag() { // timing_info_present_flag
			// timing_info()
			r.skipBits(32)    // num_units_in_display_tick
			r.skipBits(32)    // time_scale
			if r.readFlag() { // equal_picture_interval
				r.readUVLC() // num_ticks_per_picture_minus_1
			}

			if decoderModelInfo = r.readFlag(); decoderModelInfo {
				// decoder_model_info()
				bufferDelayLength = r.readBits(5) + 1
				r.skipBits(32) // num_units_in_decoding_tick
				r.skipBits(5)  // buffer_removal_time_length_minus_1
				r.skipBits(5)  // frame_presentation_time_length_minus_1
			}
		}

		initialDisplayDelay = r.readFlag()

		opCount := r.readBits(5) + 1
		for i := uint32(0); i < opCount; i++ {
			r.skipBits(12) // operating_point_idc

			level := byte(r.readBits(5))
			var tier byte
			if level > 7 {
				tier = byte(r.readBits(1))
			}
			if i == 0 {
				info.Level, info.Tier = level, tier
			}

			if decoderModelInfo && r.readFlag() { // decoder_model_present_for_this_op
				// operating_parameters_info()
				r.skipBits(bufferDelayLength) // decoder_buffer_delay
				r.skipBits(bufferDelayLength) // encoder_buffer_delay
				r.skipBits(1)                 // low_delay_mode_flag
			}

			if initialDisplayDelay && r.readFlag() { // initial_display_delay_present_for_this_op
				r.skipBits(4) // initial_display_delay_minus_1
			}

			if r.eof {
				return nil
			}
		}
	}

	// frame dimensions, present for reduced_still_picture_header too
	widthBits := r.readBits(4) + 1
	heightBits := r.readBits(4) + 1
	info.Width = clampDimension(r.readBits(widthBits))
	info.Height = clampDimension(r.readBits(heightBits))

	if !reducedStill && r.readFlag() { // frame_id_numbers_present_flag
		r.skipBits(4) // delta_frame_id_length_minus_2
		r.skipBits(3) // additional_frame_id_length_minus_1
	}

	r.skipBits(1) // use_128x128_superblock
	r.skipBits(1) // enable_filter_intra
	r.skipBits(1) // enable_intra_edge_filter

	if !reducedStill {
		r.skipBits(1) // enable_interintra_compound
		r.skipBits(1) // enable_masked_compound
		r.skipBits(1) // enable_warped_motion
		r.skipBits(1) // enable_dual_filter

		enableOrderHint := r.readFlag()
		if enableOrderHint {
			r.skipBits(1) // enable_jnt_comp
			r.skipBits(1) // enable_ref_frame_mvs
		}

		// SELECT_SCREEN_CONTENT_TOOLS when seq_choose_screen_content_tools is set
		forceScreenContent := uint32(2)
		if !r.readFlag() { // seq_choose_screen_content_tools
			forceScreenContent = r.readBits(1)
		}
		if forceScreenContent > 0 {
			if !r.readFlag() { // seq_choose_integer_mv
				r.skipBits(1) // seq_force_integer_mv
			}
		}

		if enableOrderHint {
			r.skipBits(3) // order_hint_bits_minus_1
		}
	}

	r.skipBits(1) // enable_superres
	r.skipBits(1) // enable_cdef
	r.skipBits(1) // enable_restoration

	parseColorConfig(r, info)

	if r.eof {
		return nil
	}

	return info
}

// parseColorConfig follows color_config() from AV1 spec Section 5.5.2.
func parseColorConfig(r *bitReader, info *SequenceHeaderInfo) {
	info.highBitdepth = byte(r.readBits(1))

	info.BitDepth = 8
	switch {
	case info.Profile == 2 && info.highBitdepth == 1:
		info.twelveBit = byte(r.readBits(1))
		if info.twelveBit == 1 {
			info.BitDepth = 12
		} else {
			info.BitDepth = 10
		}
	case info.highBitdepth == 1:
		info.BitDepth = 10
	}

	if info.Profile != 1 {
		info.Monochrome = r.readFlag()
	}

	// CP_UNSPECIFIED, TC_UNSPECIFIED, MC_UNSPECIFIED
	colorPrimaries, transfer, matrix := uint32(2), uint32(2), uint32(2)
	if r.readFlag() { // color_description_present_flag
		colorPrimaries = r.readBits(8)
		transfer = r.readBits(8)
		matrix = r.readBits(8)
	}

	if info.Monochrome {
		r.skipBits(1) // color_range
		info.ChromaSubsamplingX = 1
		info.ChromaSubsamplingY = 1
		return
	}

	if colorPrimaries == cpBT709 && transfer == tcSRGB && matrix == mcIdentity {
		// implicit full range 4:4:4, no color_range bit
		r.skipBits(1) // separate_uv_delta_q
		return
	}

	r.skipBits(1) // color_range

	switch {
	case info.Profile == 0:
		info.ChromaSubsamplingX = 1
		info.ChromaSubsamplingY = 1
	case info.Profile == 1:
		// 4:4:4, both stay 0
	case info.BitDepth == 12:
		info.ChromaSubsamplingX = byte(r.readBits(1))
		if info.ChromaSubsamplingX == 1 {
			info.ChromaSubsamplingY = byte(r.readBits(1))
		}
	default:
		info.ChromaSubsamplingX = 1 // 4:2:2
	}

	if info.ChromaSubsamplingX == 1 && info.ChromaSubsamplingY == 1 {
		info.ChromaSamplePos = byte(r.readBits(2))
	}

	r.skipBits(1) // separate_uv_delta_q
}

// MimeCodecString generates the av01 MIME codec string from a raw Sequence Header OBU.
// Format: av01.<profile>.<levelIdx><tier>.<bitDepth>
// See: https://aomediacodec.github.io/av1-isobmff/#codecsparam
func MimeCodecString(seqHdr []byte) string {
	info := ParseSequenceHeaderInfo(seqHdr)
	if info == nil {
		return ""
	}

	tier := 'M'
	if info.Tier == 1 {
		tier = 'H'
	}

	return fmt.Sprintf("av01.%d.%02d%c.%02d", info.Profile, info.Level, tier, info.BitDepth)
}

// EncodeConfig creates an AV1CodecConfigurationRecord for the av1C box in MP4.
// See: https://aomediacodec.github.io/av1-isobmff/#av1codecconfigurationrecord
//
// The seqHdr should be a raw Sequence Header OBU (with OBU header and size).
// If seqHdr can't be parsed, a default config (Main profile, level 4.0, 8-bit,
// 4:2:0) is used, so the muxer still produces a playable init segment.
func EncodeConfig(seqHdr []byte) []byte {
	info := ParseSequenceHeaderInfo(seqHdr)
	if info == nil {
		info = &SequenceHeaderInfo{Level: 8, ChromaSubsamplingX: 1, ChromaSubsamplingY: 1}
	}

	var monochrome byte
	if info.Monochrome {
		monochrome = 1
	}

	conf := []byte{
		0x81, // marker=1, version=1
		(info.Profile << 5) | (info.Level & 0x1F),
		(info.Tier << 7) | (info.highBitdepth << 6) | (info.twelveBit << 5) | (monochrome << 4) |
			(info.ChromaSubsamplingX << 3) | (info.ChromaSubsamplingY << 2) | (info.ChromaSamplePos & 0x03),
		0x00, // reserved(3)=0, initial_presentation_delay_present=0, reserved(4)=0
	}

	return append(conf, seqHdr...)
}

// ConfigToCodec parses an AV1CodecConfigurationRecord (av1C) into a core.Codec.
// The record is carried in the enhanced-RTMP/FLV PacketTypeSequenceStart body
// and in the ISOBMFF av1C box.
func ConfigToCodec(conf []byte) *core.Codec {
	codec := &core.Codec{
		Name:        core.CodecAV1,
		ClockRate:   90000,
		PayloadType: core.PayloadTypeRAW,
	}

	// configOBUs, everything after the 4 byte record header
	if len(conf) > 4 {
		if seqHdr := SequenceHeader(conf[4:]); seqHdr != nil {
			codec.FmtpLine = EncodeFmtpLine(seqHdr)
		}
	}

	return codec
}

// DecodeSequenceHeader parses a Sequence Header OBU and returns width and height.
// Convenience wrapper around ParseSequenceHeaderInfo.
func DecodeSequenceHeader(obu []byte) (width, height uint16) {
	if info := ParseSequenceHeaderInfo(obu); info != nil {
		return info.Width, info.Height
	}
	return 0, 0
}

// AV1 has no fmtp parameter carrying the sequence header (RFC 9583), so go2rtc
// uses its own, shaped like the H264/H265 sprop parameters.
const fmtpSeqHeader = "sprop-seq-header="

// EncodeFmtpLine stores a raw Sequence Header OBU in a codec FmtpLine.
func EncodeFmtpLine(seqHdr []byte) string {
	return fmtpSeqHeader + base64.StdEncoding.EncodeToString(seqHdr)
}

// GetSequenceHeader extracts the raw Sequence Header OBU from a codec FmtpLine.
func GetSequenceHeader(fmtp string) []byte {
	s := core.Between(fmtp, fmtpSeqHeader, ";")
	if s == "" {
		return nil
	}
	seqHdr, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(seqHdr) == 0 {
		return nil
	}
	return seqHdr
}

// clampDimension converts max_frame_*_minus_1 to a dimension. The spec allows up
// to 16 bits, so the +1 can overflow the uint16 the MP4 track header uses.
func clampDimension(minusOne uint32) uint16 {
	if minusOne >= 0xFFFF {
		return 0xFFFF
	}
	return uint16(minusOne + 1)
}

// bitReader is a simple bit-level reader. Once it runs past the end of the data
// it sets eof and returns zeros, so callers can reject truncated headers instead
// of acting on garbage.
type bitReader struct {
	data []byte
	pos  int // bit offset
	eof  bool
}

func (r *bitReader) readBits(n uint32) uint32 {
	if r.eof || r.pos+int(n) > len(r.data)*8 {
		r.eof = true
		return 0
	}

	var val uint32
	for i := uint32(0); i < n; i++ {
		val = val<<1 | uint32(r.data[r.pos>>3]>>(7-r.pos&7)&1)
		r.pos++
	}
	return val
}

func (r *bitReader) readFlag() bool {
	return r.readBits(1) == 1
}

func (r *bitReader) skipBits(n uint32) {
	if r.eof || r.pos+int(n) > len(r.data)*8 {
		r.eof = true
		return
	}
	r.pos += int(n)
}

// readUVLC follows uvlc() from AV1 spec Section 4.10.3: count zeros up to the
// terminating one bit, and only then decide whether the value fits.
func (r *bitReader) readUVLC() uint32 {
	var leadingZeros uint32
	for !r.readFlag() {
		if r.eof {
			return 0
		}
		leadingZeros++
	}

	if leadingZeros >= 32 {
		return 0xFFFFFFFF
	}
	if leadingZeros == 0 {
		return 0
	}

	return (1<<leadingZeros - 1) + r.readBits(leadingZeros)
}
