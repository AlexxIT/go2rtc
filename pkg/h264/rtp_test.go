package h264

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRTPDepayFUA(t *testing.T) {
	// Create a FU-A fragment. The FU header contains the NAL unit
	// type and the Start (0x80) and End (0x40) bits.
	fu := func(seq uint16, ts uint32, header byte, data ...byte) *rtp.Packet {
		return &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: seq,
				Timestamp:      ts,
				Marker:         header&0x40 != 0,
			},
			Payload: append([]byte{0x60 | fuaNALUType, header}, data...),
		}
	}

	// Create a non-fragmented NAL unit.
	nal := func(seq uint16, ts uint32, marker bool, data ...byte) *rtp.Packet {
		return &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: seq,
				Timestamp:      ts,
				Marker:         marker,
			},
			Payload: data,
		}
	}

	tests := []struct {
		name    string
		packets []*rtp.Packet
		want    [][]byte
	}{
		{
			name: "valid_fragments",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11), // start
				fu(11, 100, 0x01, 0x22), // middle
				fu(12, 100, 0x41, 0x33), // end
			},
			want: [][]byte{
				{0, 0, 0, 4, 0x61, 0x11, 0x22, 0x33},
			},
		},
		{
			name: "missing_start",
			packets: []*rtp.Packet{
				// A consumer joins in the middle of an IDR.
				fu(10, 100, 0x05, 0x11),
				fu(11, 100, 0x45, 0x22),
			},
			want: nil,
		},
		{
			name: "missing_start_recovery",
			packets: []*rtp.Packet{
				// Ignore the incomplete IDR.
				fu(10, 100, 0x05, 0x11),
				fu(11, 100, 0x45, 0x22),

				// Accept the next complete NAL unit.
				fu(12, 200, 0x81, 0x33),
				fu(13, 200, 0x41, 0x44),
			},
			want: [][]byte{
				{0, 0, 0, 3, 0x61, 0x33, 0x44},
			},
		},
		{
			name: "missing_end",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),

				// A new start must discard the unfinished NAL
				// even if sequence numbers are continuous.
				fu(11, 200, 0x81, 0x22),
				fu(12, 200, 0x41, 0x33),
			},
			want: [][]byte{
				{0, 0, 0, 3, 0x61, 0x22, 0x33},
			},
		},
		{
			name: "missing_middle",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),
				// Sequence number 11 is missing.
				fu(12, 100, 0x41, 0x33),
			},
			want: nil,
		},
		{
			name: "missing_middle_recovery",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),
				fu(12, 100, 0x41, 0x33),

				// The next NAL must not contain stale fragments.
				fu(13, 200, 0x81, 0x44),
				fu(14, 200, 0x41, 0x55),
			},
			want: [][]byte{
				{0, 0, 0, 3, 0x61, 0x44, 0x55},
			},
		},
		{
			name: "out_of_order",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),
				fu(12, 100, 0x01, 0x33),
				fu(11, 100, 0x01, 0x22),
				fu(13, 100, 0x41, 0x44),
			},
			want: nil,
		},
		{
			name: "duplicate_fragment",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),
				fu(11, 100, 0x01, 0x22),
				fu(11, 100, 0x01, 0x22),
				fu(12, 100, 0x41, 0x33),
			},
			want: nil,
		},
		{
			name: "sequence_number_wraparound",
			packets: []*rtp.Packet{
				fu(65534, 100, 0x81, 0x11),
				fu(65535, 100, 0x01, 0x22),
				fu(0, 100, 0x41, 0x33),
			},
			want: [][]byte{
				{0, 0, 0, 4, 0x61, 0x11, 0x22, 0x33},
			},
		},
		{
			name: "interrupted_by_single_nal",
			packets: []*rtp.Packet{
				fu(10, 100, 0x81, 0x11),

				// A non-fragmented NAL interrupts the FU-A.
				nal(11, 200, true, 0x61, 0x22),

				// The old FU-A must no longer be active.
				fu(12, 100, 0x41, 0x33),

				// Verify that fragmentation still works.
				fu(13, 300, 0x81, 0x44),
				fu(14, 300, 0x41, 0x55),
			},
			want: [][]byte{
				{0, 0, 0, 2, 0x61, 0x22},
				{0, 0, 0, 3, 0x61, 0x44, 0x55},
			},
		},
		{
			name: "discard_incomplete_access_unit",
			packets: []*rtp.Packet{
				// Buffer a NAL without the marker bit.
				nal(10, 100, false, 0x61, 0x11),

				// The next NAL is fragmented and damaged.
				fu(11, 100, 0x81, 0x22),
				fu(13, 100, 0x41, 0x33),

				// The previous access unit must be discarded.
				nal(14, 200, true, 0x61, 0x44),
			},
			want: [][]byte{
				{0, 0, 0, 2, 0x61, 0x44},
			},
		},
		{
			name: "single_nal",
			packets: []*rtp.Packet{
				nal(10, 100, true, 0x61, 0x11),
				nal(11, 200, true, 0x61, 0x22),
			},
			want: [][]byte{
				{0, 0, 0, 2, 0x61, 0x11},
				{0, 0, 0, 2, 0x61, 0x22},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][]byte

			handler := RTPDepay(
				&core.Codec{
					Name:      core.CodecH264,
					ClockRate: 90000,
				},
				func(packet *core.Packet) {
					require.Equal(t, uint8(RTPPacketVersionAVC), packet.Version)

					// Copy the payload because RTPDepay may reuse its buffer.
					got = append(got, append([]byte(nil), packet.Payload...))
				},
			)

			for _, packet := range tt.packets {
				handler(packet)
			}

			require.Equal(t, tt.want, got)
		})
	}
}
