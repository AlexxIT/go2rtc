package ps

import (
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/h264/golomb"
)

const firstByte = 0x67

// Google to "h264 specification pdf"
// https://www.itu.int/rec/dologin_pub.asp?lang=e&id=T-REC-H.264-201602-S!!PDF-E&type=items

type SPS struct {
	Profile    string
	ProfileIDC uint8
	ProfileIOP uint8
	LevelIDC   uint8
	Width      uint16
	Height     uint16
}

func NewSPS(profile string, level uint8, width uint16, height uint16) *SPS {
	s := &SPS{
		Profile: profile, LevelIDC: level, Width: width, Height: height,
	}
	s.ProfileIDC, s.ProfileIOP = DecodeProfile(profile)
	return s
}

// https://www.cardinalpeak.com/blog/the-h-264-sequence-parameter-set

func (s *SPS) Marshal() []byte {
	w := golomb.NewWriter()

	// this is typical SPS for most H264 cameras
	w.WriteByte(firstByte)
	w.WriteByte(s.ProfileIDC)
	w.WriteByte(s.ProfileIOP)
	w.WriteByte(s.LevelIDC)

	w.WriteUEGolomb(0) // seq_parameter_set_id (0)
	w.WriteUEGolomb(0) // log2_max_frame_num_minus4 (depends)
	w.WriteUEGolomb(0) // pic_order_cnt_type (0 or 2)
	w.WriteUEGolomb(0) // log2_max_pic_order_cnt_lsb_minus4 (depends)
	w.WriteUEGolomb(1) // num_ref_frames (1)
	w.WriteBit(0)      // gaps_in_frame_num_value_allowed_flag (0)

	w.WriteUEGolomb(uint8(s.Width>>4) - 1)  // pic_width_in_mbs_minus_1
	w.WriteUEGolomb(uint8(s.Height>>4) - 1) // pic_height_in_map_units_minus_1

	w.WriteBit(1) // frame_mbs_only_flag (1)
	w.WriteBit(1) // direct_8x8_inference_flag (1)
	w.WriteBit(0) // frame_cropping_flag (0 is OK)
	w.WriteBit(0) // vui_prameters_present_flag (0 is OK)
	w.WriteBit(1) // rbsp_stop_one_bit

	return w.Bytes()
}

func (s *SPS) Unmarshal(data []byte) (err error) {
	r := golomb.NewReader(data)

	var b byte
	var u uint

	if b, err = r.ReadByte(); err != nil {
		return
	}
	if b&0x1F != 7 {
		err = errors.New("not SPS data")
		return
	}

	if s.ProfileIDC, err = r.ReadByte(); err != nil {
		return
	}
	if s.ProfileIOP, err = r.ReadByte(); err != nil {
		return
	}
	if s.LevelIDC, err = r.ReadByte(); err != nil {
		return
	}

	s.Profile = EncodeProfile(s.ProfileIDC, s.ProfileIOP)

	u, err = r.ReadUEGolomb() // seq_parameter_set_id

	if s.ProfileIDC == 100 || s.ProfileIDC == 110 || s.ProfileIDC == 122 ||
		s.ProfileIDC == 244 || s.ProfileIDC == 44 || s.ProfileIDC == 83 ||
		s.ProfileIDC == 86 || s.ProfileIDC == 118 || s.ProfileIDC == 128 ||
		s.ProfileIDC == 138 || s.ProfileIDC == 139 || s.ProfileIDC == 134 ||
		s.ProfileIDC == 135 {
		var n byte

		u, err = r.ReadUEGolomb() // chroma_format_idc
		if u == 3 {
			b, err = r.ReadBit() // separate_colour_plane_flag
			n = 12
		} else {
			n = 8
		}

		u, err = r.ReadUEGolomb() // bit_depth_luma_minus8
		u, err = r.ReadUEGolomb() // bit_depth_chroma_minus8
		b, err = r.ReadBit()      // qpprime_y_zero_transform_bypass_flag

		b, err = r.ReadBit() // seq_scaling_matrix_present_flag
		if b > 0 {
			for i := byte(0); i < n; i++ {
				b, err = r.ReadBit() // seq_scaling_list_present_flag[i]
				if b > 0 {
					panic("not implemented")
				}
			}
		}
	}

	u, err = r.ReadUEGolomb() // log2_max_frame_num_minus4

	u, err = r.ReadUEGolomb() // pic_order_cnt_type
	switch u {
	case 0:
		u, err = r.ReadUEGolomb() // log2_max_pic_order_cnt_lsb_minus4
	case 1:
		b, err = r.ReadBit()      // delta_pic_order_always_zero_flag
		_, err = r.ReadSEGolomb() // offset_for_non_ref_pic
		_, err = r.ReadSEGolomb() // offset_for_top_to_bottom_field
		u, err = r.ReadUEGolomb() // num_ref_frames_in_pic_order_cnt_cycle
		for i := byte(0); i < b; i++ {
			_, err = r.ReadSEGolomb() // offset_for_ref_frame[i]
		}
	}

	u, err = r.ReadUEGolomb() // num_ref_frames
	b, err = r.ReadBit()      // gaps_in_frame_num_value_allowed_flag

	u, err = r.ReadUEGolomb() // pic_width_in_mbs_minus_1
	s.Width = uint16(u+1) << 4
	u, err = r.ReadUEGolomb() // pic_height_in_map_units_minus_1
	s.Height = uint16(u+1) << 4

	b, err = r.ReadBit() // frame_mbs_only_flag
	if b == 0 {
		_, err = r.ReadBit()
	}

	b, err = r.ReadBit() // direct_8x8_inference_flag

	b, err = r.ReadBit() // frame_cropping_flag
	if b > 0 {
		u, err = r.ReadUEGolomb() // frame_crop_left_offset
		s.Width -= uint16(u) << 1
		u, err = r.ReadUEGolomb() // frame_crop_right_offset
		s.Width -= uint16(u) << 1
		u, err = r.ReadUEGolomb() // frame_crop_top_offset
		s.Height -= uint16(u) << 1
		u, err = r.ReadUEGolomb() // frame_crop_bottom_offset
		s.Height -= uint16(u) << 1
	}

	b, err = r.ReadBit() // vui_prameters_present_flag
	if b > 0 {
		b, err = r.ReadBit() // vui_prameters_present_flag
		if b > 0 {
			u, err = r.ReadBits(8) // aspect_ratio_idc
			if b == 255 {
				u, err = r.ReadBits(16) // sar_width
				u, err = r.ReadBits(16) // sar_height
			}
		}

		b, err = r.ReadBit() // overscan_info_present_flag
		if b > 0 {
			b, err = r.ReadBit() // overscan_appropriate_flag
		}

		b, err = r.ReadBit() // video_signal_type_present_flag
		if b > 0 {
			u, err = r.ReadBits(3) // video_format
			b, err = r.ReadBit()   // video_full_range_flag

			b, err = r.ReadBit() // colour_description_present_flag
			if b > 0 {
				u, err = r.ReadBits(8) // colour_primaries
				u, err = r.ReadBits(8) // transfer_characteristics
				u, err = r.ReadBits(8) // matrix_coefficients
			}
		}

		b, err = r.ReadBit() // chroma_loc_info_present_flag
		if b > 0 {
			u, err = r.ReadUEGolomb() // chroma_sample_loc_type_top_field
			u, err = r.ReadUEGolomb() // chroma_sample_loc_type_bottom_field
		}

		b, err = r.ReadBit() // timing_info_present_flag
		if b > 0 {
			u, err = r.ReadBits(32) // num_units_in_tick
			u, err = r.ReadBits(32) // time_scale
			b, err = r.ReadBit()    // fixed_frame_rate_flag
		}

		b, err = r.ReadBit() // nal_hrd_parameters_present_flag
		if b > 0 {
			//panic("not implemented")
			return nil
		}

		b, err = r.ReadBit() // vcl_hrd_parameters_present_flag
		if b > 0 {
			//panic("not implemented")
			return nil
		}

		// if (nal_hrd_parameters_present_flag || vcl_hrd_parameters_present_flag)
		//     b, err = r.ReadBit() // low_delay_hrd_flag

		b, err = r.ReadBit() // pic_struct_present_flag

		b, err = r.ReadBit() // bitstream_restriction_flag
		if b > 0 {
			b, err = r.ReadBit()      // motion_vectors_over_pic_boundaries_flag
			u, err = r.ReadUEGolomb() // max_bytes_per_pic_denom
			u, err = r.ReadUEGolomb() // max_bits_per_mb_denom
			u, err = r.ReadUEGolomb() // log2_max_mv_length_horizontal
			u, err = r.ReadUEGolomb() // log2_max_mv_length_vertical
			u, err = r.ReadUEGolomb() // max_num_reorder_frames
			u, err = r.ReadUEGolomb() // max_dec_frame_buffering
		}
	}

	b, err = r.ReadBit() // rbsp_stop_one_bit

	return
}

func EncodeProfile(idc, iop byte) string {
	// https://datatracker.ietf.org/doc/html/rfc6184#page-41
	switch {
	// 4240xx 42C0xx 42E0xx
	case idc == 0x42 && iop&0b01001111 == 0b01000000:
		return "CB"
	case idc == 0x4D && iop&0b10001111 == 0b10000000:
		return "CB"
	case idc == 0x58 && iop&0b11001111 == 0b11000000:
		return "CB"
	// 4200xx
	case idc == 0x42 && iop&0b01001111 == 0:
		return "B"
	case idc == 0x58 && iop&0b11001111 == 0b10000000:
		return "B"
	// 4d40xx
	case idc == 0x4D && iop&0b10101111 == 0:
		return "M"
	case idc == 0x58 && iop&0b11001111 == 0:
		return "E"
	case idc == 0x64 && iop == 0:
		return "H"
	case idc == 0x6E && iop == 0:
		return "H10"
	}
	return ""
}

func DecodeProfile(profile string) (idc, iop byte) {
	switch profile {
	case "CB":
		return 0x42, 0b01000000
	case "B":
		return 0x42, 0 // 66
	case "M":
		return 0x4D, 0 // 77
	case "E":
		return 0x58, 0 // 88
	case "H":
		return 0x64, 0
	}
	return 0, 0
}
