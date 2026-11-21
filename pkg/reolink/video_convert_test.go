package reolink

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
)

func TestEncodeOwnedToAVCCCodecAwarePaths(t *testing.T) {
	sps, pps, idr := []byte{0x47, 0x64, 0, 0x1f}, []byte{0x68, 0xee}, []byte{0x65, 0x80}
	want := avccBytesForNALUs(sps, pps, idr)
	input := h264AccessUnit(sps, pps, idr)
	got := encodeOwnedToAVCC("H264", input)
	if !bytes.Equal(got, want) || len(got) == 0 || &got[0] != &input[0] {
		t.Fatalf("four-byte H264 path did not preserve SPS in place: got=%x", got)
	}

	threeByte := []byte{0, 0, 1}
	threeByte = append(threeByte, sps...)
	threeByte = append(threeByte, 0, 0, 1)
	threeByte = append(threeByte, pps...)
	threeByte = append(threeByte, 0, 0, 1)
	threeByte = append(threeByte, idr...)
	checkCodecFallback(t, "H264 three-byte", "H264", threeByte, want)

	withAUD := h264AccessUnit([]byte{0x09, 0xf0}, sps, pps, idr)
	checkCodecFallback(t, "H264 AUD", "H264", withAUD, want)
}

func TestEncodeOwnedToAVCCFallbackPreservesInput(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
		want []byte
	}{
		{
			name: "late three-byte prefix",
			data: []byte{0, 0, 0, 1, 0x41, 1, 0, 0, 1, 0x42, 2},
			want: avccBytesForNALUs([]byte{0x41, 1}, []byte{0x42, 2}),
		},
		{name: "empty NAL", data: []byte{0, 0, 0, 1}},
		{
			name: "trailing prefix",
			data: []byte{0, 0, 0, 1, 0x41, 1, 0, 0, 1},
			want: avccBytesForNALUs([]byte{0x41, 1, 0, 0, 1}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkCodecFallback(t, test.name, "H264", test.data, test.want)
		})
	}
}

func TestEncodeOwnedToAVCCRemovesOnlyCodecAUD(t *testing.T) {
	nalus := [][]byte{
		{0x40, 1}, {0x42, 1}, {0x44, 1},
		{0x09, 1, 0}, // HEVC type 4 with nuh_layer_id 32; not an AUD.
		{0x26, 1, 0x80}, {0x26, 1, 0},
	}
	input := testH265KeyframeData(append([][]byte{{0x46, 1}}, nalus...)...)
	checkCodecFallback(t, "H265 AUD and slices", "H265", input, avccBytesForNALUs(nalus...))
}

func TestEncodeOwnedToAVCCRejectsNonzeroPrefix(t *testing.T) {
	for _, prefix := range [][]byte{{0x55}, {0, 0x55}, {0, 0, 0x55, 0x55}} {
		input := append(append([]byte(nil), prefix...), 0, 0, 0, 1, 0x41, 3)
		if got := encodeOwnedToAVCC("H264", input); len(got) != 0 {
			t.Fatalf("accepted nonzero Annex-B prefix: %x -> %x", input, got)
		}
	}
}

func FuzzEncodeOwnedToAVCC(f *testing.F) {
	for _, input := range [][]byte{
		{0, 0, 0, 1, 0x41, 1, 2, 0, 0, 0, 1, 0x41, 3},
		{0, 0, 0, 1, 0x46, 0x64, 0, 0x1f, 0, 0, 0, 1, 0x41, 1},
		{0, 0, 0, 1, 0x47, 0x64, 0, 0x1f, 0, 0, 0, 1, 0x41, 1},
		{0, 0, 1, 0xc7},
		{0, 0, 1, 0x41, 1, 2},
		{0, 0, 0, 1, 0x09, 0xf0, 0, 0, 0, 1, 0x41, 3},
		{0, 0, 0, 1, 0, 0, 0, 1, 0x41, 3},
		{0x55, 0, 0, 0, 1, 0x41, 3},
	} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			t.Skip()
		}
		oracleInput := normalizeH264ParentAUDHeaders(input)
		parentInput, ownedInput := bytes.Clone(oracleInput), bytes.Clone(oracleInput)
		want := annexb.EncodeToAVCC(parentInput)
		if start := bytes.Index(oracleInput, []byte{0, 0, 1}); start >= 0 && len(bytes.Trim(oracleInput[:start], "\x00")) != 0 {
			want = nil
		}
		if got := encodeOwnedToAVCC("H264", ownedInput); !bytes.Equal(got, want) {
			t.Fatalf("unexpected normalized differential: %x != %x", got, want)
		}
	})
}

func FuzzCodecAwareAVCCPreservesNALBytes(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 1, 0x46, 0x47, 0x09, 0})
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 1<<16 {
			t.Skip()
		}
		for _, test := range []struct {
			codec  string
			header []byte
		}{
			{"H264", []byte{0x46, 0x64, 0, 0x1f}},
			{"H264", []byte{0x47, 0x64, 0, 0x1f}},
			{"H264", []byte{0xc6, 0x64, 0, 0x1f}},
			{"H264", []byte{0xc7, 0x64, 0, 0x1f}},
			{"H265", []byte{0x09, 0x01, 0x80}},
		} {
			nalu := append(bytes.Clone(test.header), escapeNALPayload(payload)...)
			checkCodecNALBytes(t, test.codec, []byte{0, 0, 0, 1}, nalu, true)
			checkCodecNALBytes(t, test.codec, []byte{0, 0, 1}, nalu, false)
		}
	})
}

func normalizeH264ParentAUDHeaders(data []byte) []byte {
	// Normalize all aliases away from the parent's H265 AUD mask for this oracle only.
	data = bytes.Clone(data)
	for i := 0; i+2 < len(data); i++ {
		codeLen := 0
		if data[i] == 0 && data[i+1] == 0 {
			if data[i+2] == 1 {
				codeLen = 3
			} else if data[i+2] == 0 && i+3 < len(data) && data[i+3] == 1 {
				codeLen = 4
			}
		}
		if codeLen != 0 {
			header := i + codeLen
			if header < len(data) && data[header]&0x7e == 35<<1 {
				data[header] = data[header]&0x1f | 0x60
			}
		}
	}
	return data
}

func escapeNALPayload(payload []byte) []byte {
	data := make([]byte, 0, len(payload)+len(payload)/16)
	zeros := 0
	for _, value := range payload {
		if zeros >= 2 && value <= 3 {
			data = append(data, 3)
			zeros = 0
		}
		data = append(data, value)
		if value == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return data
}

func checkCodecNALBytes(t *testing.T, codec string, prefix, nalu []byte, inPlace bool) {
	t.Helper()
	input := append(bytes.Clone(prefix), nalu...)
	original := bytes.Clone(input)
	want := binary.BigEndian.AppendUint32(nil, uint32(len(nalu)))
	want = append(want, nalu...)
	got := encodeOwnedToAVCC(codec, input)
	if !bytes.Equal(got, want) || len(got) == 0 || (&got[0] == &input[0]) != inPlace {
		t.Fatalf("%s NAL bytes changed: %x -> %x", codec, nalu, got)
	}
	if !inPlace && !bytes.Equal(input, original) {
		t.Fatalf("%s fallback modified source bytes", codec)
	}
}

func checkCodecFallback(t *testing.T, name, codec string, input, want []byte) {
	t.Helper()
	original := bytes.Clone(input)
	got := encodeOwnedToAVCC(codec, input)
	if !bytes.Equal(got, want) {
		t.Fatalf("%s conversion = %x, want %x", name, got, want)
	}
	if len(got) != 0 && &got[0] == &input[0] {
		t.Fatalf("%s fallback reused packet storage", name)
	}
	if !bytes.Equal(input, original) {
		t.Fatalf("%s fallback modified packet storage", name)
	}
}

func avccBytesForNALUs(nalus ...[]byte) []byte {
	var data []byte
	for _, nalu := range nalus {
		data = binary.BigEndian.AppendUint32(data, uint32(len(nalu)))
		data = append(data, nalu...)
	}
	return data
}

func testH265KeyframeData(nalus ...[]byte) []byte {
	data := make([]byte, 0, len(nalus)*6)
	for _, nalu := range nalus {
		data = append(data, 0, 0, 0, 1)
		data = append(data, nalu...)
	}
	return data
}
