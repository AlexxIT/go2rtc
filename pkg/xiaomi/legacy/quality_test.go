package legacy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestG2Quality(t *testing.T) {
	for in, want := range map[string]g2Level{
		"":    {"fhd", 0, 1080},
		"fhd": {"fhd", 0, 1080},
		"0":   {"fhd", 0, 1080},
		"hd":  {"hd", 1, 720},
		"1":   {"hd", 1, 720},
		"sd":  {"sd", 2, 360},
		"2":   {"sd", 2, 360},
	} {
		got, ok := g2Quality(in)
		require.True(t, ok, in)
		require.Equal(t, want, got, in)
	}

	_, ok := g2Quality("auto")
	require.False(t, ok)
}

func TestSetQualityErrors(t *testing.T) {
	err := SetQuality("nobody", "hd")
	require.ErrorContains(t, err, "no running session")

	active.Store("other", &Producer{client: &Client{model: "isa.camera.hlc7"}})
	t.Cleanup(func() { active.Delete("other") })
	require.ErrorIs(t, SetQuality("other", "hd"), ErrQualityUnsupported)

	active.Store("g2", &Producer{client: &Client{model: ModelAqaraG2}})
	t.Cleanup(func() { active.Delete("g2") })
	require.ErrorIs(t, SetQuality("g2", "auto"), ErrQualityUnsupported)
}
