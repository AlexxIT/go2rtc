package h264

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/bits"
)

// http://www.itu.int/rec/T-REC-H.264
// https://webrtc.googlesource.com/src/+/refs/heads/main/common_video/h264/sps_parser.cc

//goland:noinspection GoSnakeCaseUsage
type SPS struct {
	profile_idc uint8
	profile_iop uint8
	level_idc   uint8

	seq_parameter_set_id uint32

	chroma_format_idc                    uint32
	separate_colour_plane_flag           byte
	bit_depth_luma_minus8                uint32
	bit_depth_chroma_minus8              uint32
	qpprime_y_zero_transform_bypass_flag byte
	seq_scaling_matrix_present_flag      byte

	log2_max_frame_num_minus4             uint32
	pic_order_cnt_type                    uint32
	log2_max_pic_order_cnt_lsb_minus4     uint32
	delta_pic_order_always_zero_flag      byte
	offset_for_non_ref_pic                int32
	offset_for_top_to_bottom_field        int32
	num_ref_frames_in_pic_order_cnt_cycle uint32
	num_ref_frames                        uint32
	gaps_in_frame_num_value_allowed_flag  byte

	pic_width_in_mbs_minus_1        uint32
	pic_height_in_map_units_minus_1 uint32
	frame_mbs_only_flag             byte
	mb_adaptive_frame_field_flag    byte
	direct_8x8_inference_flag       byte

	frame_cropping_flag      byte
	frame_crop_left_offset   uint32
	frame_crop_right_offset  uint32
	frame_crop_top_offset    uint32
	frame_crop_bottom_offset uint32

	vui_parameters_present_flag    byte
	aspect_ratio_info_present_flag byte
	aspect_ratio_idc               byte
	sar_width                      uint16
	sar_height                     uint16

	overscan_info_present_flag byte
	overscan_appropriate_flag  byte

	video_signal_type_present_flag byte
	video_format                   uint8
	video_full_range_flag          byte

	colour_description_present_flag byte
	colour_description              uint32

	chroma_loc_info_present_flag        byte
	chroma_sample_loc_type_top_field    uint32
	chroma_sample_loc_type_bottom_field uint32

	timing_info_present_flag byte
	num_units_in_tick        uint32
	time_scale               uint32
	fixed_frame_rate_flag    byte
}

func (s *SPS) Width() uint16 {
	width := 16 * (s.pic_width_in_mbs_minus_1 + 1)
	crop := 2 * (s.frame_crop_left_offset + s.frame_crop_right_offset)
	return uint16(width - crop)
}

func (s *SPS) Height() uint16 {
	height := 16 * (s.pic_height_in_map_units_minus_1 + 1)
	crop := 2 * (s.frame_crop_top_offset + s.frame_crop_bottom_offset)
	if s.frame_mbs_only_flag == 0 {
		height *= 2
	}
	return uint16(height - crop)
}

func DecodeSPS(sps []byte) *SPS {
	r := bits.NewReader(sps)

	hdr := r.ReadByte()
	if hdr&0x1F != NALUTypeSPS {
		return nil
	}

	s := &SPS{
		profile_idc:          r.ReadByte(),
		profile_iop:          r.ReadByte(),
		level_idc:            r.ReadByte(),
		seq_parameter_set_id: r.ReadUEGolomb(),
	}

	switch s.profile_idc {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		n := byte(8)

		s.chroma_format_idc = r.ReadUEGolomb()
		if s.chroma_format_idc == 3 {
			s.separate_colour_plane_flag = r.ReadBit()
			n = 12
		}

		s.bit_depth_luma_minus8 = r.ReadUEGolomb()
		s.bit_depth_chroma_minus8 = r.ReadUEGolomb()
		s.qpprime_y_zero_transform_bypass_flag = r.ReadBit()

		s.seq_scaling_matrix_present_flag = r.ReadBit()
		if s.seq_scaling_matrix_present_flag != 0 {
			for i := byte(0); i < n; i++ {
				//goland:noinspection GoSnakeCaseUsage
				seq_scaling_list_present_flag := r.ReadBit()
				if seq_scaling_list_present_flag != 0 {
					if i < 6 {
						s.scaling_list(r, 16)
					} else {
						s.scaling_list(r, 64)
					}
				}
			}
		}
	}

	s.log2_max_frame_num_minus4 = r.ReadUEGolomb()

	s.pic_order_cnt_type = r.ReadUEGolomb()
	switch s.pic_order_cnt_type {
	case 0:
		s.log2_max_pic_order_cnt_lsb_minus4 = r.ReadUEGolomb()
	case 1:
		s.delta_pic_order_always_zero_flag = r.ReadBit()
		s.offset_for_non_ref_pic = r.ReadSEGolomb()
		s.offset_for_top_to_bottom_field = r.ReadSEGolomb()

		s.num_ref_frames_in_pic_order_cnt_cycle = r.ReadUEGolomb()
		for i := uint32(0); i < s.num_ref_frames_in_pic_order_cnt_cycle; i++ {
			_ = r.ReadSEGolomb() // offset_for_ref_frame[i]
		}
	}

	s.num_ref_frames = r.ReadUEGolomb()
	s.gaps_in_frame_num_value_allowed_flag = r.ReadBit()

	s.pic_width_in_mbs_minus_1 = r.ReadUEGolomb()
	s.pic_height_in_map_units_minus_1 = r.ReadUEGolomb()

	s.frame_mbs_only_flag = r.ReadBit()
	if s.frame_mbs_only_flag == 0 {
		s.mb_adaptive_frame_field_flag = r.ReadBit()
	}

	s.direct_8x8_inference_flag = r.ReadBit()

	s.frame_cropping_flag = r.ReadBit()
	if s.frame_cropping_flag != 0 {
		s.frame_crop_left_offset = r.ReadUEGolomb()
		s.frame_crop_right_offset = r.ReadUEGolomb()
		s.frame_crop_top_offset = r.ReadUEGolomb()
		s.frame_crop_bottom_offset = r.ReadUEGolomb()
	}

	s.vui_parameters_present_flag = r.ReadBit()
	if s.vui_parameters_present_flag != 0 {
		s.aspect_ratio_info_present_flag = r.ReadBit()
		if s.aspect_ratio_info_present_flag != 0 {
			s.aspect_ratio_idc = r.ReadByte()
			if s.aspect_ratio_idc == 255 {
				s.sar_width = r.ReadUint16()
				s.sar_height = r.ReadUint16()
			}
		}

		s.overscan_info_present_flag = r.ReadBit()
		if s.overscan_info_present_flag != 0 {
			s.overscan_appropriate_flag = r.ReadBit()
		}

		s.video_signal_type_present_flag = r.ReadBit()
		if s.video_signal_type_present_flag != 0 {
			s.video_format = r.ReadBits8(3)
			s.video_full_range_flag = r.ReadBit()

			s.colour_description_present_flag = r.ReadBit()
			if s.colour_description_present_flag != 0 {
				s.colour_description = r.ReadUint24()
			}
		}

		s.chroma_loc_info_present_flag = r.ReadBit()
		if s.chroma_loc_info_present_flag != 0 {
			s.chroma_sample_loc_type_top_field = r.ReadUEGolomb()
			s.chroma_sample_loc_type_bottom_field = r.ReadUEGolomb()
		}

		s.timing_info_present_flag = r.ReadBit()
		if s.timing_info_present_flag != 0 {
			s.num_units_in_tick = r.ReadUint32()
			s.time_scale = r.ReadUint32()
			s.fixed_frame_rate_flag = r.ReadBit()
		}
		//...
	}

	if r.EOF {
		return nil
	}

	return s
}

//goland:noinspection GoSnakeCaseUsage
func (s *SPS) scaling_list(r *bits.Reader, sizeOfScalingList int) {
	lastScale := int32(8)
	nextScale := int32(8)
	for j := 0; j < sizeOfScalingList; j++ {
		if nextScale != 0 {
			delta_scale := r.ReadSEGolomb()
			nextScale = (lastScale + delta_scale + 256) % 256
		}
		if nextScale != 0 {
			lastScale = nextScale
		}
	}
}

func (s *SPS) Profile() string {
	switch s.profile_idc {
	case 0x42:
		return "Baseline"
	case 0x4D:
		return "Main"
	case 0x58:
		return "Extended"
	case 0x64:
		return "High"
	}
	return fmt.Sprintf("0x%02X", s.profile_idc)
}

func (s *SPS) PixFmt() string {
	if s.bit_depth_luma_minus8 == 0 {
		switch s.chroma_format_idc {
		case 1:
			if s.video_full_range_flag == 1 {
				return "yuvj420p"
			}
			return "yuv420p"
		case 2:
			return "yuv422p"
		case 3:
			return "yuv444p"
		}
	}
	return ""
}

func (s *SPS) String() string {
	return fmt.Sprintf(
		"%s %d.%d, %s, %dx%d",
		s.Profile(), s.level_idc/10, s.level_idc%10, s.PixFmt(), s.Width(), s.Height(),
	)
}

// FixPixFmt - change yuvj420p to yuv420p in SPS
// same as "-c:v copy -bsf:v h264_metadata=video_full_range_flag=0"
func FixPixFmt(sps []byte) {
	r := bits.NewReader(sps)

	_ = r.ReadByte()

	profile := r.ReadByte()
	_ = r.ReadByte()
	_ = r.ReadByte()
	_ = r.ReadUEGolomb()

	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		n := byte(8)

		if r.ReadUEGolomb() == 3 {
			_ = r.ReadBit()
			n = 12
		}

		_ = r.ReadUEGolomb()
		_ = r.ReadUEGolomb()
		_ = r.ReadBit()

		if r.ReadBit() != 0 {
			for i := byte(0); i < n; i++ {
				if r.ReadBit() != 0 {
					return // skip
				}
			}
		}
	}

	_ = r.ReadUEGolomb()

	switch r.ReadUEGolomb() {
	case 0:
		_ = r.ReadUEGolomb()
	case 1:
		_ = r.ReadBit()
		_ = r.ReadSEGolomb()
		_ = r.ReadSEGolomb()

		n := r.ReadUEGolomb()
		for i := uint32(0); i < n; i++ {
			_ = r.ReadSEGolomb()
		}
	}

	_ = r.ReadUEGolomb()
	_ = r.ReadBit()

	_ = r.ReadUEGolomb()
	_ = r.ReadUEGolomb()

	if r.ReadBit() == 0 {
		_ = r.ReadBit()
	}

	_ = r.ReadBit()

	if r.ReadBit() != 0 {
		_ = r.ReadUEGolomb()
		_ = r.ReadUEGolomb()
		_ = r.ReadUEGolomb()
		_ = r.ReadUEGolomb()
	}

	if r.ReadBit() != 0 {
		if r.ReadBit() != 0 {
			if r.ReadByte() == 255 {
				_ = r.ReadUint16()
				_ = r.ReadUint16()
			}
		}

		if r.ReadBit() != 0 {
			_ = r.ReadBit()
		}

		if r.ReadBit() != 0 {
			_ = r.ReadBits8(3)
			if r.ReadBit() == 1 {
				pos, bit := r.Pos()
				sps[pos] &= ^byte(1 << bit)
			}
		}
	}
}
