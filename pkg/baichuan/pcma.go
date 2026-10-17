package baichuan

// EncodePCMA converts 16-bit linear PCM to 8-bit A-law.
func EncodePCMA(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, sample := range pcm {
		out[i] = linearToALaw(sample)
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

