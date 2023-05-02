package v1

import (
	v2 "github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPCMUtoPCM(t *testing.T) {
	for pcmu := byte(0); pcmu < 255; pcmu++ {
		pcm1 := MuLawDecompressTable[pcmu]
		pcm2 := v2.PCMUtoPCM(pcmu)
		require.Equal(t, pcm1, pcm2)
	}
}

func TestPCMAtoPCM(t *testing.T) {
	for pcma := byte(0); pcma < 255; pcma++ {
		pcm1 := ALawDecompressTable[pcma]
		pcm2 := v2.PCMAtoPCM(pcma)
		require.Equal(t, pcm1, pcm2)
	}
}

func TestPCMtoPCMU(t *testing.T) {
	for pcm := int16(-32768); pcm < 32767; pcm++ {
		pcmu1 := LinearToMuLawSample(pcm)
		pcmu2 := v2.PCMtoPCMU(pcm)
		require.Equal(t, pcmu1, pcmu2)
	}
}

func TestPCMtoPCMA(t *testing.T) {
	for pcm := int16(-32768); pcm < 32767; pcm++ {
		pcma1 := LinearToALawSample(pcm)
		pcma2 := v2.PCMtoPCMA(pcm)
		require.Equal(t, pcma1, pcma2)
	}
}
