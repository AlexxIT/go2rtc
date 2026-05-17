package baichuan

// EncodePCMA converts 16-bit linear PCM to 8-bit A-law.
func EncodePCMA(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, sample := range pcm {
		out[i] = linearToALaw(sample)
	}
	return out
}

// DecodePCMA converts 8-bit A-law to 16-bit linear PCM.
func DecodePCMA(data []byte) []int16 {
	out := make([]int16, len(data))
	for i, v := range data {
		out[i] = aLawToLinear(v)
	}
	return out
}

// DecodePCMU converts 8-bit mu-law to 16-bit linear PCM.
func DecodePCMU(data []byte) []int16 {
	out := make([]int16, len(data))
	for i, v := range data {
		out[i] = muLawToLinear(v)
	}
	return out
}

func linearToALaw(pcm int16) byte {
	var sign int16
	var exponent int16
	var mantissa int16
	var alaw byte

	if pcm >= 0 {
		sign = 0x80
	} else {
		sign = 0x00
		pcm = -pcm - 1
	}

	// pcm is an int16, so it can never be greater than 32767.
	// We handle the overflow case before this function.
	// if pcm > 32767 {
	// 	pcm = 32767
	// }

	if pcm >= 256 {
		exponent = 7
		for (pcm & 0x4000) == 0 {
			pcm <<= 1
			exponent--
		}
		mantissa = (pcm >> 10) & 0x0F
		alaw = byte(sign | (exponent << 4) | mantissa) //#nosec G115
	} else {
		alaw = byte(sign | ((pcm >> 4) & 0x0F)) //#nosec G115
	}

	return alaw ^ 0x55
}

func aLawToLinear(v byte) int16 {
	v ^= 0x55

	t := int16(v&0x0F) << 4
	seg := (v & 0x70) >> 4
	switch seg {
	case 0:
		t += 8
	case 1:
		t += 0x108
	default:
		t += 0x108
		t <<= seg - 1
	}

	if (v & 0x80) == 0 {
		return -t
	}
	return t
}

func muLawToLinear(v byte) int16 {
	v = ^v

	t := ((int(v) & 0x0F) << 3) + 0x84
	t <<= (uint(v) & 0x70) >> 4
	if (v & 0x80) != 0 {
		return int16(t - 0x84) //#nosec G115
	}
	return int16(0x84 - t) //#nosec G115
}
