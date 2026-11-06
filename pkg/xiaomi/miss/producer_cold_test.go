package miss

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/h265"
)

func accessUnit(nalus ...[]byte) []byte {
	var result []byte
	for _, nalu := range nalus {
		result = binary.BigEndian.AppendUint32(result, uint32(len(nalu)))
		result = append(result, nalu...)
	}
	return result
}

func TestInitialParameterSetsAfterPrefix(t *testing.T) {
	if !containsNALU(accessUnit([]byte{35 << 1, 1}, []byte{32 << 1, 1}), 32, true) {
		t.Fatal("HEVC VPS after AUD must be recognized in the first access unit")
	}
	if !containsNALU(accessUnit([]byte{9, 1}, []byte{7, 1}), 7, false) {
		t.Fatal("H264 SPS after AUD must be recognized in the first access unit")
	}
	if containsNALU([]byte{0, 0, 2, 0, 64}, 32, true) {
		t.Fatal("truncated parameter set must not be accepted")
	}
}

func TestParameterSetsReleaseWithoutAudio(t *testing.T) {
	var split parameterSets
	split.absorb(accessUnit([]byte{35 << 1, 1}, []byte{32 << 1, 1}), true)
	if split.ready() {
		t.Fatal("VPS alone must not release")
	}
	split.absorb(accessUnit([]byte{33 << 1, 1, 2}), true)
	if !split.ready() || split.label() != "vps+sps" {
		t.Fatal("VPS+SPS must release before audio and before PPS")
	}
	split.absorb(accessUnit([]byte{34 << 1, 9}), true)
	codec := h265.AVCCToCodec(split.avcc)
	if !strings.Contains(codec.FmtpLine, "sprop-vps=") || !strings.Contains(codec.FmtpLine, "sprop-sps=") || !strings.Contains(codec.FmtpLine, "sprop-pps=") {
		t.Fatalf("split parameter sets were not kept: %s", codec.FmtpLine)
	}

	var together parameterSets
	together.absorb(accessUnit([]byte{32 << 1, 1}, []byte{33 << 1, 2}, []byte{34 << 1, 3}, []byte{19 << 1, 4}), true)
	if !together.ready() || together.label() != "vps+sps+pps" {
		t.Fatal("one access unit with VPS+SPS+PPS must release immediately")
	}

	var h264sets parameterSets
	h264sets.absorb(accessUnit([]byte{9, 1}, []byte{7, 2}), false)
	if h264sets.ready() {
		t.Fatal("H264 SPS alone must not release")
	}
	h264sets.absorb(accessUnit([]byte{8, 3}), false)
	if !h264sets.ready() || h264sets.label() != "sps+pps" {
		t.Fatal("H264 must release at SPS+PPS")
	}
}
