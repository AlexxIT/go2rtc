package h265

import (
	"bytes"

	"github.com/AlexxIT/go2rtc/pkg/bits"
)

// http://www.itu.int/rec/T-REC-H.265

//goland:noinspection GoSnakeCaseUsage
type SPS struct {
	sps_video_parameter_set_id   uint8
	sps_max_sub_layers_minus1    uint8
	sps_temporal_id_nesting_flag byte

	general_profile_space               uint8
	general_tier_flag                   byte
	general_profile_idc                 uint8
	general_profile_compatibility_flags uint32

	general_level_idc              uint8
	sub_layer_profile_present_flag []byte
	sub_layer_level_present_flag   []byte

	sps_seq_parameter_set_id   uint32
	chroma_format_idc          uint32
	separate_colour_plane_flag byte

	pic_width_in_luma_samples  uint32
	pic_height_in_luma_samples uint32

	conformance_window_flag byte
	conf_win_left_offset    uint32
	conf_win_right_offset   uint32
	conf_win_top_offset     uint32
	conf_win_bottom_offset  uint32
}

// Width returns the picture width after cropping by the conformance window:
// the displayed width rather than the coded width.
func (s *SPS) Width() uint16 {
	// Encoders code pictures in whole blocks (1080 lines are often coded as 1088)
	// and declare the excess as the conformance window (H.265 7.4.3.2.1).
	// Its offsets count in units of SubWidthC and SubHeightC luma samples, which
	// depend on the chroma format (H.265 Table 6-1):
	//
	//	chroma_format_idc  format      SubWidthC  SubHeightC
	//	0                  monochrome  1          1
	//	1                  4:2:0       2          2
	//	2                  4:2:2       2          1
	//	3                  4:4:4       1          1
	crop := s.conf_win_left_offset + s.conf_win_right_offset
	if s.chroma_format_idc == 1 || s.chroma_format_idc == 2 {
		crop *= 2 // SubWidthC
	}
	if crop >= s.pic_width_in_luma_samples {
		return uint16(s.pic_width_in_luma_samples) // invalid window (7.4.3.2.1), ignore it
	}
	return uint16(s.pic_width_in_luma_samples - crop)
}

// Height returns the picture height after cropping by the conformance window:
// the displayed height rather than the coded height.
func (s *SPS) Height() uint16 {
	crop := s.conf_win_top_offset + s.conf_win_bottom_offset
	if s.chroma_format_idc == 1 {
		crop *= 2 // SubHeightC (see Width)
	}
	if crop >= s.pic_height_in_luma_samples {
		return uint16(s.pic_height_in_luma_samples) // invalid window (7.4.3.2.1), ignore it
	}
	return uint16(s.pic_height_in_luma_samples - crop)
}

func DecodeSPS(nalu []byte) *SPS {
	rbsp := bytes.ReplaceAll(nalu[2:], []byte{0, 0, 3}, []byte{0, 0})

	r := bits.NewReader(rbsp)
	s := &SPS{}

	s.sps_video_parameter_set_id = r.ReadBits8(4)
	s.sps_max_sub_layers_minus1 = r.ReadBits8(3)
	s.sps_temporal_id_nesting_flag = r.ReadBit()

	if !s.profile_tier_level(r) {
		return nil
	}

	s.sps_seq_parameter_set_id = r.ReadUEGolomb()
	s.chroma_format_idc = r.ReadUEGolomb()
	if s.chroma_format_idc == 3 {
		s.separate_colour_plane_flag = r.ReadBit()
	}

	s.pic_width_in_luma_samples = r.ReadUEGolomb()
	s.pic_height_in_luma_samples = r.ReadUEGolomb()

	s.conformance_window_flag = r.ReadBit()
	if s.conformance_window_flag != 0 {
		s.conf_win_left_offset = r.ReadUEGolomb()
		s.conf_win_right_offset = r.ReadUEGolomb()
		s.conf_win_top_offset = r.ReadUEGolomb()
		s.conf_win_bottom_offset = r.ReadUEGolomb()
	}

	//...

	if r.EOF {
		return nil
	}

	return s
}

// profile_tier_level supports ONLY general_profile_idc == 1
// over variants very complicated...
//
//goland:noinspection GoSnakeCaseUsage
func (s *SPS) profile_tier_level(r *bits.Reader) bool {
	s.general_profile_space = r.ReadBits8(2)
	s.general_tier_flag = r.ReadBit()
	s.general_profile_idc = r.ReadBits8(5)

	s.general_profile_compatibility_flags = r.ReadBits(32)
	_ = r.ReadBits64(48) // other flags

	if s.general_profile_idc != 1 {
		return false
	}

	s.general_level_idc = r.ReadBits8(8)

	s.sub_layer_profile_present_flag = make([]byte, s.sps_max_sub_layers_minus1)
	s.sub_layer_level_present_flag = make([]byte, s.sps_max_sub_layers_minus1)

	for i := byte(0); i < s.sps_max_sub_layers_minus1; i++ {
		s.sub_layer_profile_present_flag[i] = r.ReadBit()
		s.sub_layer_level_present_flag[i] = r.ReadBit()
	}

	if s.sps_max_sub_layers_minus1 > 0 {
		for i := s.sps_max_sub_layers_minus1; i < 8; i++ {
			_ = r.ReadBits8(2) // reserved_zero_2bits
		}
	}

	for i := byte(0); i < s.sps_max_sub_layers_minus1; i++ {
		if s.sub_layer_profile_present_flag[i] != 0 {
			_ = r.ReadBits8(2)                      // sub_layer_profile_space
			_ = r.ReadBit()                         // sub_layer_tier_flag
			sub_layer_profile_idc := r.ReadBits8(5) // sub_layer_profile_idc

			_ = r.ReadBits(32)   // sub_layer_profile_compatibility_flag
			_ = r.ReadBits64(48) // other flags

			if sub_layer_profile_idc != 1 {
				return false
			}
		}

		if s.sub_layer_level_present_flag[i] != 0 {
			_ = r.ReadBits8(8)
		}
	}

	return true
}
