package shell

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommandCloseWaitsForExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command test uses POSIX sh")
	}

	cmd := NewCommand(`sh -c "sleep 30"`)
	require.NoError(t, cmd.Start())

	start := time.Now()
	err := cmd.Close()
	require.Less(t, time.Since(start), 7*time.Second)
	require.Error(t, err)
	require.NotNil(t, cmd.ProcessState)
}
