package reolink

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
)

// encodeOwnedToAVCC reuses access units with nonempty NALs, four-byte start codes, and no AUD.
// Preview owns packet data; every other layout uses the parent converter.
func encodeOwnedToAVCC(data []byte) []byte {
	if !canEncodeToAVCCInPlace(data) {
		return annexb.EncodeToAVCC(data)
	}
	header, nalu := 0, 4
	for {
		offset := bytes.Index(data[nalu:], []byte{0, 0, 1})
		if offset < 0 {
			binary.BigEndian.PutUint32(data[header:], uint32(len(data)-nalu))
			return data
		}
		next := nalu + offset - 1
		binary.BigEndian.PutUint32(data[header:], uint32(next-nalu))
		header, nalu = next, next+4
	}
}

func canEncodeToAVCCInPlace(data []byte) bool {
	if len(data) < 5 || binary.BigEndian.Uint32(data) != 1 {
		return false
	}
	for nalu := 4; ; {
		if nalu >= len(data) || data[nalu]&0x1f == 9 || data[nalu]&0x7e == 35<<1 {
			return false
		}
		offset := bytes.Index(data[nalu:], []byte{0, 0, 1})
		if offset < 0 {
			return true
		}
		marker := nalu + offset
		if marker <= nalu+1 || data[marker-1] != 0 {
			return false
		}
		nalu = marker + 3
	}
}

func validVideoConfig(codec string, data []byte) bool {
	info, err := inspectVideoPayload(codec, data)
	return err == nil && info.keyframe && info.fullParameters
}

type videoPayload struct {
	keyframe       bool
	hasParameters  bool
	fullParameters bool
}

func inspectVideoPayload(codec string, data []byte) (videoPayload, error) {
	minNALU := 1
	if codec == "H265" {
		minNALU = 2
	} else if codec != "H264" {
		return videoPayload{}, fmt.Errorf("unsupported codec %q", codec)
	}
	if len(data) < 4+minNALU {
		return videoPayload{}, fmt.Errorf("invalid AVCC framing")
	}
	var first, second, third, keyframe bool
	index := 0
	for len(data) != 0 {
		if len(data) < 4 {
			return videoPayload{}, fmt.Errorf("invalid AVCC framing")
		}
		size := binary.BigEndian.Uint32(data)
		if size < uint32(minNALU) || uint64(size) > uint64(len(data)-4) {
			return videoPayload{}, fmt.Errorf("invalid AVCC framing")
		}
		nalu := data[4 : 4+int(size)]
		switch codec {
		case "H264":
			typeID := nalu[0] & 0x1f
			if nalu[0]&0x80 != 0 {
				return videoPayload{}, fmt.Errorf("NAL %d has forbidden bit", index)
			}
			if typeID == 0 || typeID >= 24 {
				return videoPayload{}, fmt.Errorf("NAL %d has type %d", index, typeID)
			}
			switch typeID {
			case h264.NALUTypeSPS:
				if len(nalu) < 4 {
					return videoPayload{}, nil
				}
				first = true
			case h264.NALUTypePPS:
				second = true
			case h264.NALUTypeIFrame:
				keyframe = true
			}
		case "H265":
			typeID := (nalu[0] >> 1) & 0x3f
			if nalu[0]&0x80 != 0 {
				return videoPayload{}, fmt.Errorf("NAL %d has forbidden bit", index)
			}
			if typeID >= 48 {
				return videoPayload{}, fmt.Errorf("NAL %d has type %d", index, typeID)
			}
			if nalu[1]&7 == 0 {
				return videoPayload{}, fmt.Errorf("NAL %d has temporal ID zero", index)
			}
			switch typeID {
			case h265.NALUTypeVPS:
				first = true
			case h265.NALUTypeSPS:
				second = true
			case h265.NALUTypePPS:
				third = true
			case h265.NALUTypeIFrame, h265.NALUTypeIFrame2, h265.NALUTypeIFrame3:
				keyframe = true
			}
		}
		data = data[4+int(size):]
		index++
	}
	return videoPayload{
		keyframe: keyframe, hasParameters: first || second || third,
		fullParameters: first && second && (codec == "H264" || third),
	}, nil
}

func videoParameterSignature(codec string, data []byte) (string, error) {
	const maxH264Sets = 64
	// Bound distinct encoded sets, not harmless repetitions.
	const maxH265Sets = 64 * (16 + 16 + 64)
	minSize := uint32(1)
	var required uint64
	switch codec {
	case "H264":
		required = uint64(1)<<h264.NALUTypeSPS | uint64(1)<<h264.NALUTypePPS
	case "H265":
		minSize = 2
		required = uint64(1)<<h265.NALUTypeVPS | uint64(1)<<h265.NALUTypeSPS | uint64(1)<<h265.NALUTypePPS
	default:
		return "", mediaErrorf("reolink: unsupported video codec %q", codec)
	}
	sets := make([]string, 0, 3)
	var found uint64
	h264Count := 0
	var h265Sets map[[sha256.Size + 1]byte]struct{}
	for len(data) >= 4 {
		size := binary.BigEndian.Uint32(data)
		if size < minSize || uint64(size) > uint64(len(data)-4) {
			return "", mediaErrorf("reolink: invalid %s parameter sets", codec)
		}
		nalu := data[4 : 4+int(size)]
		typeID := nalu[0] & 0x1f
		if codec == "H265" {
			typeID = (nalu[0] >> 1) & 0x3f
		}
		setMask := uint64(1) << typeID
		if required&setMask != 0 {
			if codec == "H264" {
				if h264Count == maxH264Sets {
					return "", mediaErrorf("reolink: H264 parameter sets exceed %d", maxH264Sets)
				}
				h264Count++
			}
			found |= setMask
			// Parameter-set RBSPs end with a nonzero trailing-bits byte. These
			// source Annex-B trailing zeros are padding, not parameter identity.
			for len(nalu) > int(minSize) && nalu[len(nalu)-1] == 0 {
				nalu = nalu[:len(nalu)-1]
			}
			digest := sha256.Sum256(nalu)
			var raw [sha256.Size + 1]byte
			raw[0] = typeID
			copy(raw[1:], digest[:])
			if codec == "H265" {
				if h265Sets == nil {
					h265Sets = make(map[[sha256.Size + 1]byte]struct{})
				}
				if _, ok := h265Sets[raw]; !ok {
					if len(h265Sets) == maxH265Sets {
						return "", mediaErrorf("reolink: H265 parameter sets exceed %d", maxH265Sets)
					}
					h265Sets[raw] = struct{}{}
					sets = append(sets, string(raw[:]))
				}
			} else {
				key := string(raw[:])
				duplicate := false
				for _, existing := range sets {
					if existing == key {
						duplicate = true
						break
					}
				}
				if !duplicate {
					sets = append(sets, key)
				}
			}
		}
		data = data[4+int(size):]
	}
	if len(data) != 0 || found != required {
		return "", mediaErrorf("reolink: invalid %s parameter sets", codec)
	}
	sort.Strings(sets)
	return strings.Join(sets, ""), nil
}
