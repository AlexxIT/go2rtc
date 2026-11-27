package mjpeg

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewYCbCr(
		image.Rect(0, 0, w, h),
		image.YCbCrSubsampleRatio420,
	)

	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

func TestTranscode(t *testing.T) {
	for _, tt := range []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"standard", 1920, 1080, 1920, 1080},
		{"unaligned_width_1", 641, 480, 641, 480},
		{"unaligned_width_2", 642, 480, 642, 480},
		{"unaligned_width_3", 643, 480, 643, 480},
		{"unaligned_height", 640, 481, 640, 481},
		{"unaligned_both", 641, 483, 641, 483},
		{"small", 3, 5, 3, 5},
		{"max_size", 2040, 2040, 2040, 2040},
		{"over_max_width", 2041, 480, 2040, 480},
		{"over_max_height", 640, 2041, 640, 2040},
		{"large", 2560, 1440, 2040, 1440},
		{"large_both", 2560, 2160, 2040, 2040},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := makeJPEG(t, tt.w, tt.h)

			b, err := Transcode(b)
			require.NoError(t, err)

			img, err := jpeg.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, image.Pt(tt.wantW, tt.wantH),
				img.Bounds().Size())
		})
	}
}

func TestTranscodeInvalid(t *testing.T) {
	for _, b := range [][]byte{
		nil,
		{},
		{0xFF},
		{0xFF, 0xD8},
	} {
		_, err := Transcode(b)
		require.Error(t, err)
	}
}

func TestRTPPay(t *testing.T) {
	for _, tt := range []struct {
		name         string
		w, h         int
		wantW, wantH uint8
	}{
		{"standard", 640, 480, 80, 60},
		{"unaligned_width", 647, 480, 80, 60},
		{"unaligned_height", 640, 487, 80, 60},
		{"unaligned_both", 647, 487, 80, 60},
		{"next_block", 648, 488, 81, 61},
		{"max_size", 2040, 2040, 255, 255},
		{"large", 2560, 1440, 255, 180},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := 0

			handler := RTPPay(func(packet *rtp.Packet) {
				require.GreaterOrEqual(t, len(packet.Payload), 8)
				require.Equal(t, tt.wantW, packet.Payload[6])
				require.Equal(t, tt.wantH, packet.Payload[7])
				n++
			})

			handler(&rtp.Packet{
				Payload: makeJPEG(t, tt.w, tt.h),
			})

			require.Positive(t, n)
		})
	}
}

func TestRTPDepaySize(t *testing.T) {
	for _, tt := range []struct {
		w, h         uint8
		wantW, wantH int
	}{
		{80, 60, 640, 480},
		{255, 255, 2040, 2040},
		{64, 240, 2560, 1920},
		{64, 180, 2560, 1440},
		{224, 14, 3840, 2160},
		{32, 162, 2304, 1296},
	} {
		var payload []byte

		handler := RTPDepay(func(packet *rtp.Packet) {
			payload = bytes.Clone(packet.Payload)
		})

		handler(&rtp.Packet{
			Header: rtp.Header{Marker: true},
			Payload: []byte{
				0, 0, 0, 0,
				1, 50, tt.w, tt.h,
			},
		})

		require.NotEmpty(t, payload)

		conf, err := jpeg.DecodeConfig(bytes.NewReader(payload))
		require.NoError(t, err)
		require.Equal(t, tt.wantW, conf.Width)
		require.Equal(t, tt.wantH, conf.Height)
	}
}

func TestRTPDepayEOI(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want []byte
	}{
		{
			"present",
			[]byte{0xFF, 0xD9},
			[]byte{0xFF, 0xD9},
		},
		{
			"missing",
			[]byte{0x12, 0x34},
			[]byte{0x12, 0x34, 0xFF, 0xD9},
		},
		{
			"partial_ff",
			[]byte{0xFF, 0x00},
			[]byte{0xFF, 0x00, 0xFF, 0xD9},
		},
		{
			"partial_d9",
			[]byte{0x12, 0xD9},
			[]byte{0x12, 0xD9, 0xFF, 0xD9},
		},
		{
			"single_byte",
			[]byte{0x12},
			[]byte{0x12, 0xFF, 0xD9},
		},
		{
			"empty",
			nil,
			[]byte{0xFF, 0xD9},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var payload []byte

			handler := RTPDepay(func(packet *rtp.Packet) {
				payload = bytes.Clone(packet.Payload)
			})

			handler(&rtp.Packet{
				Header: rtp.Header{Marker: true},
				Payload: append(
					[]byte{0, 0, 0, 0, 1, 50, 80, 60},
					tt.data...,
				),
			})

			lqt, cqt := MakeTables(50)
			want := MakeHeaders(nil, 1, 640, 480, lqt, cqt)
			want = append(want, tt.want...)

			require.Equal(t, want, payload)
		})
	}
}
