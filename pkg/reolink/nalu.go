package reolink

func splitAnnexB(buf []byte) [][]byte {
	var out [][]byte
	var start int
	var found bool

	for i := 0; i < len(buf)-3; i++ {
		prefixLen := startCodeLen(buf[i:])
		if prefixLen == 0 {
			continue
		}

		if found && i > start {
			out = append(out, cloneBytes(buf[start:i]))
		}
		start = i + prefixLen
		found = true
		i += prefixLen - 1
	}

	if found && start < len(buf) {
		out = append(out, cloneBytes(buf[start:]))
	}

	if len(out) == 0 && len(buf) > 0 {
		out = append(out, cloneBytes(buf))
	}

	trimmed := out[:0]
	for _, nalu := range out {
		if len(nalu) > 0 {
			trimmed = append(trimmed, nalu)
		}
	}
	return trimmed
}

func filterH265DecodableNALs(nalus [][]byte) [][]byte {
	out := nalus[:0]
	for _, n := range nalus {
		if len(n) < 2 {
			continue
		}
		t := (n[0] >> 1) & 0x3F
		if t >= 48 {
			continue
		}
		layerID := ((n[0] & 1) << 5) | (n[1] >> 3)
		if layerID != 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

func h265NALUnitType(header0 byte) byte {
	return (header0 >> 1) & 0x3F
}

func h265IsSliceNAL(typ byte) bool {
	return typ <= 9 || (typ >= 16 && typ <= 21)
}

func reorderH265NALsForAccessUnit(nalus [][]byte) [][]byte {
	var nonSlice, slice [][]byte
	for _, n := range nalus {
		if len(n) < 2 {
			continue
		}
		if h265IsSliceNAL(h265NALUnitType(n[0])) {
			slice = append(slice, n)
		} else {
			nonSlice = append(nonSlice, n)
		}
	}
	return append(nonSlice, slice...)
}

func startCodeLen(buf []byte) int {
	if len(buf) >= 4 && buf[0] == 0 && buf[1] == 0 && buf[2] == 0 && buf[3] == 1 {
		return 4
	}
	if len(buf) >= 3 && buf[0] == 0 && buf[1] == 0 && buf[2] == 1 {
		return 3
	}
	return 0
}

func cloneBytes(buf []byte) []byte {
	return append([]byte(nil), buf...)
}
