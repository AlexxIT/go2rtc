package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatchConfigReadOnly(t *testing.T) {
	prevPath := ConfigPath
	prevReadOnly := ConfigReadOnly
	t.Cleanup(func() {
		ConfigPath = prevPath
		ConfigReadOnly = prevReadOnly
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0644))

	ConfigPath = path
	ConfigReadOnly = true

	err := PatchConfig([]string{"streams", "cam"}, "rtsp://example.com")
	require.Error(t, err)
	require.EqualError(t, err, "config is read-only")
}
