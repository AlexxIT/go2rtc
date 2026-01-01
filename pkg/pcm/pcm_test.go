package pcm

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestTranscode(t *testing.T) {
	tests := []struct {
		name   string
		src    core.Codec
		dst    core.Codec
		source string
		expect string
	}{
		{
			name:   "s16be->s16be",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			source: "FCCA00130343062808130B510D9E0F7610DA111113EA15BD16F2168215D41561",
			expect: "FCCA00130343062808130B510D9E0F7610DA111113EA15BD16F2168215D41561",
		},
		{
			name:   "s16be->s16le",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCML, ClockRate: 8000, Channels: 1},
			source: "FCCA00130343062808130B510D9E0F7610DA111113EA15BD16F2168215D41561",
			expect: "CAFC1300430328061308510B9E0D760FDA101111EA13BD15F2168216D4156115",
		},
		{
			name:   "s16be->mulaw",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1},
			source: "FCCA00130343062808130B510D9E0F7610DA111113EA15BD16F2168215D41561",
			expect: "52FDD1C5BEB8B3B0AEAEABA9A8A8A9AA",
		},
		{
			name:   "s16be->alaw",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1},
			source: "FCCA00130343062808130B510D9E0F7610DA111113EA15BD16F2168215D41561",
			expect: "7CD4FFED95939E9B8584868083838080",
		},
		{
			name:   "2ch->1ch",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 2},
			dst:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			source: "FCCAFCCA001300130343034306280628081308130B510B510D9E0D9E0F760F76",
			expect: "FCCA00130343062808130B510D9E0F76",
		},
		{
			name:   "1ch->2ch",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 2},
			source: "FCCA00130343062808130B510D9E0F76",
			expect: "FCCAFCCA001300130343034306280628081308130B510B510D9E0D9E0F760F76",
		},
		{
			name:   "16khz->8khz",
			src:    core.Codec{Name: core.CodecPCM, ClockRate: 16000, Channels: 1},
			dst:    core.Codec{Name: core.CodecPCM, ClockRate: 8000, Channels: 1},
			source: "FCCAFCCA001300130343034306280628081308130B510B510D9E0D9E0F760F76",
			expect: "FCCA00130343062808130B510D9E0F76",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := Transcode(&test.dst, &test.src)
			b, _ := hex.DecodeString(test.source)
			b = f(b)
			s := fmt.Sprintf("%X", b)
			require.Equal(t, test.expect, s)
		})
	}
}
