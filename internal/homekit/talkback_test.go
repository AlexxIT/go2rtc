package homekit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEncoderProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the encoder")
	}

	// Stand-in encoder: records its arguments, emits two framed units, then
	// waits for the end of its input
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	path := filepath.Join(dir, "enc")
	script := "#!/bin/sh\necho \"$@\" > " + args + "\nprintf '\\000\\003abc\\000\\001z'\ncat > /dev/null\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))

	frames := make(chan string, 2)
	enc, err := startEncoder(path, 16000, 480, func(au []byte) {
		frames <- string(au)
	})
	require.NoError(t, err)

	for _, want := range []string{"abc", "z"} {
		select {
		case got := <-frames:
			require.Equal(t, want, got)
		case <-time.After(5 * time.Second):
			t.Fatal("no frame from the encoder")
		}
	}

	_, err = enc.Write(make([]byte, 960))
	require.NoError(t, err)
	require.NoError(t, enc.Close())

	b, err := os.ReadFile(args)
	require.NoError(t, err)
	require.Equal(t, "-r 16000 -f 480\n", string(b))
}
