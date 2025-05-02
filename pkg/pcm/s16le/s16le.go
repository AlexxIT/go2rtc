package s16le

func PeaksRMS(b []byte) int16 {
	// RMS of sine wave = peak / sqrt2
	// https://en.wikipedia.org/wiki/Root_mean_square
	// https://www.youtube.com/watch?v=MUDkL4KZi0I
	var peaks int32
	var peaksSum int32
	var prevSample int16
	var prevUp bool

	var i int
	for n := len(b); i < n; {
		lo := b[i]
		i++
		hi := b[i]
		i++

		sample := int16(hi)<<8 | int16(lo)
		up := sample >= prevSample

		if i >= 4 {
			if up != prevUp {
				if prevSample >= 0 {
					peaksSum += int32(prevSample)
				} else {
					peaksSum -= int32(prevSample)
				}
				peaks++
			}
		}

		prevSample = sample
		prevUp = up
	}

	if peaks == 0 {
		return 0
	}

	return int16(peaksSum / peaks)
}
