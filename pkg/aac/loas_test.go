package aac

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// testdata:
//   - eld_16k_480.loas: eld-enc --loas (FDK AAC), 1.92 s of a 1 kHz tone
//   - lc_16k.loas: ffmpeg -c:a aac -f latm, 1 s of a 1 kHz tone

func readLOAS(t *testing.T, name string) (*LOASProducer, [][]byte) {
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	require.True(t, IsLOAS(b))

	prod, err := OpenLOAS(bytes.NewReader(b))
	require.NoError(t, err)

	units := [][]byte{prod.pending}
	for {
		unit, err := prod.readFrame()
		if err == io.EOF {
			return prod, units
		}
		require.NoError(t, err)
		units = append(units, unit)
	}
}

func TestLOASELD(t *testing.T) {
	prod, units := readLOAS(t, "eld_16k_480.loas")

	codec := prod.GetMedias()[0].Codecs[0]
	require.Equal(t, core.CodecELD, codec.Name)
	require.Equal(t, uint32(16000), codec.ClockRate)
	require.Equal(t, uint8(1), codec.Channels)
	// AAC-ELD, 16 kHz, mono, 480 samples: the config FDK AAC itself reports
	require.Equal(t, FMTP+"f8f03000", codec.FmtpLine)

	// 1.92 s / 30 ms, plus the encoder's delay
	require.GreaterOrEqual(t, len(units), 64)
	for _, unit := range units {
		require.NotEmpty(t, unit)
	}
}

func TestLOASLC(t *testing.T) {
	prod, units := readLOAS(t, "lc_16k.loas")

	codec := prod.GetMedias()[0].Codecs[0]
	require.Equal(t, core.CodecAAC, codec.Name)
	require.Equal(t, uint32(16000), codec.ClockRate)
	require.GreaterOrEqual(t, len(units), 15) // 1 s / 64 ms
}

func TestLOASWrongSync(t *testing.T) {
	_, err := OpenLOAS(bytes.NewReader([]byte{0xFF, 0xF1, 0x00, 0x00}))
	require.Error(t, err)
}
