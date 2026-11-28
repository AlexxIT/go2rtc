package reolink

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/h264"
)

// h264Fmtp reads a validated keyframe with both kinds of parameter set.
func h264Fmtp(payload []byte) string {
	var sps, pps [][]byte
	var idr []byte
	h264.EmitNalus(payload, true, func(nalu []byte) {
		var sets *[][]byte
		minSize := 2
		switch nalu[0] & 0x1f {
		case h264.NALUTypeSPS:
			sets = &sps
			minSize = 4
		case h264.NALUTypePPS:
			sets = &pps
		case h264.NALUTypeIFrame:
			if idr == nil {
				idr = nalu[1:]
			}
			return
		default:
			return
		}
		nalu = trimParameterPadding(nalu, minSize)
		if !slices.ContainsFunc(*sets, func(set []byte) bool { return bytes.Equal(set, nalu) }) {
			*sets = append(*sets, nalu)
		}
	})

	// Put the IDR's pair first for consumers that only read two sets.
	firstSPS, firstPPS := 0, 0
	if len(sps)+len(pps) > 2 {
		firstSPS, firstPPS = h264FirstPair(sps, pps, h264SlicePPS(idr))
	}

	var line strings.Builder
	line.WriteString("packetization-mode=1;profile-level-id=")
	line.WriteString(hex.EncodeToString(sps[firstSPS][1:4]))
	line.WriteString(";sprop-parameter-sets=")
	line.WriteString(base64.StdEncoding.EncodeToString(sps[firstSPS]))
	writeSet := func(nalu []byte) {
		line.WriteByte(',')
		line.WriteString(base64.StdEncoding.EncodeToString(nalu))
	}
	if firstPPS >= 0 {
		writeSet(pps[firstPPS])
	}
	for i, nalu := range sps {
		if i != firstSPS {
			writeSet(nalu)
		}
	}
	for i, nalu := range pps {
		if i != firstPPS {
			writeSet(nalu)
		}
	}
	return line.String()
}

func h264SlicePPS(idr []byte) int {
	// The first three Exp-Golomb fields fit in this prefix, even with emulation prevention.
	header := bytes.ReplaceAll(idr[:min(len(idr), 24)], []byte{0, 0, 3}, []byte{0, 0})
	r := bits.NewReader(header)
	r.ReadUEGolomb() // first_mb_in_slice
	sliceType := r.ReadUEGolomb()
	ppsID := r.ReadUEGolomb()
	if r.EOF || sliceType > 9 || ppsID > 255 {
		return -1
	}
	return int(ppsID)
}

func h264FirstPair(sps, pps [][]byte, activePPS int) (int, int) {
	firstSPS, firstPPS := 0, -1
	for i, nalu := range sps {
		r := bits.NewReader(nalu[4:])
		spsID := r.ReadUEGolomb()
		if r.EOF {
			continue
		}
		for j, nalu := range pps {
			r := bits.NewReader(nalu[1:])
			ppsID, refSPS := r.ReadUEGolomb(), r.ReadUEGolomb()
			if r.EOF || refSPS != spsID {
				continue
			}
			if activePPS < 0 || (ppsID == uint32(activePPS) && spsID <= 31) {
				return i, j
			}
			if firstPPS == -1 {
				firstSPS, firstPPS = i, j
			}
		}
	}
	return firstSPS, firstPPS
}

// h264ParametersKnown checks a validated access unit against the advertised sets.
func h264ParametersKnown(data []byte, known string) bool {
	count := 0
	for len(data) != 0 {
		size := int(binary.BigEndian.Uint32(data))
		nalu := data[4 : 4+size]
		data = data[4+size:]
		typeID := nalu[0] & 0x1f
		if typeID != h264.NALUTypeSPS && typeID != h264.NALUTypePPS {
			continue
		}
		count++
		if count > maxH264Sets {
			return false
		}
		key := videoParameterDigest(typeID, nalu, 1)
		sets := known
		for len(sets) >= len(key) && sets[:len(key)] != string(key[:]) {
			sets = sets[len(key):]
		}
		if len(sets) < len(key) {
			return false
		}
	}
	return true
}
