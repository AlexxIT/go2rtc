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
}

func (s *SPS) Width() uint16 {
	return uint16(s.pic_width_in_luma_samples)
}

func (s *SPS) Height() uint16 {
	return uint16(s.pic_height_in_luma_samples)
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
