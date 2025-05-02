package ioctl

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIOR(t *testing.T) {
	// #define SNDRV_PCM_IOCTL_INFO		_IOR('A', 0x01, struct snd_pcm_info)
	if runtime.GOARCH == "arm64" {
		c := IOR('A', 0x01, 288)
		require.Equal(t, uintptr(0x81204101), c)
	}
}
