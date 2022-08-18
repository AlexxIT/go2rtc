package ps

import (
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/h264/golomb"
)

const PPSHeader = 0x68

// https://www.itu.int/rec/T-REC-H.264
// 7.3.2.2 Picture parameter set RBSP syntax

type PPS struct{}

func (p *PPS) Marshal() []byte {
	w := golomb.NewWriter()

	// this is typical PPS for most H264 cameras
	w.WriteByte(PPSHeader)
	w.WriteUEGolomb(0) // pic_parameter_set_id
	w.WriteUEGolomb(0) // seq_parameter_set_id
	w.WriteBit(1)      // entropy_coding_mode_flag
	w.WriteBit(0)      // bottom_field_pic_order_in_frame_present_flag
	w.WriteUEGolomb(0) // num_slice_groups_minus1
	w.WriteUEGolomb(0) // num_ref_idx_l0_default_active_minus1
	w.WriteUEGolomb(0) // num_ref_idx_l1_default_active_minus1
	w.WriteBit(0)      // weighted_pred_flag
	w.WriteBits(0, 2)  // weighted_bipred_idc
	w.WriteSEGolomb(0) // pic_init_qp_minus26
	w.WriteSEGolomb(0) // pic_init_qs_minus26
	w.WriteSEGolomb(0) // chroma_qp_index_offset
	w.WriteBit(1)      // deblocking_filter_control_present_flag
	w.WriteBit(0)      // constrained_intra_pred_flag
	w.WriteBit(0)      // redundant_pic_cnt_present_flag

	w.WriteBit(1) // rbsp_trailing_bits()

	return w.Bytes()
}

func (p *PPS) Unmarshal(data []byte) (err error) {
	r := golomb.NewReader(data)

	var b byte
	var u uint

	if b, err = r.ReadByte(); err != nil {
		return
	}
	if b&0x1F != 8 {
		err = errors.New("not PPS data")
		return
	}

	// pic_parameter_set_id
	if u, err = r.ReadUEGolomb(); err != nil {
		return
	}
	// seq_parameter_set_id
	if u, err = r.ReadUEGolomb(); err != nil {
		return
	}
	// entropy_coding_mode_flag
	if b, err = r.ReadBit(); err != nil {
		return
	}
	// bottom_field_pic_order_in_frame_present_flag
	if b, err = r.ReadBit(); err != nil {
		return
	}

	// num_slice_groups_minus1
	if u, err = r.ReadUEGolomb(); err != nil {
		return
	}
	if u > 0 {
		//panic("not implemented")
		return nil
	}

	// num_ref_idx_l0_default_active_minus1
	if _, err = r.ReadUEGolomb(); err != nil {
		return
	}
	// num_ref_idx_l1_default_active_minus1
	if _, err = r.ReadUEGolomb(); err != nil {
		return
	}
	// weighted_pred_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}
	// weighted_bipred_idc
	if _, err = r.ReadBits(2); err != nil {
		return
	}
	// pic_init_qp_minus26
	if _, err = r.ReadSEGolomb(); err != nil {
		return
	}
	// pic_init_qs_minus26
	if _, err = r.ReadSEGolomb(); err != nil {
		return
	}
	// chroma_qp_index_offset
	if _, err = r.ReadSEGolomb(); err != nil {
		return
	}
	// deblocking_filter_control_present_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}
	// constrained_intra_pred_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}
	// redundant_pic_cnt_present_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}

	if !r.End() {
		//panic("not implemented")
	}

	return
}
