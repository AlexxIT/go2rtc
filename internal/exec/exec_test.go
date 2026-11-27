package exec

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPipeProbeTimeout(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	source := fmt.Sprintf(
		`exec:"%s" -test.run=^TestPipeHelperProcess$ -- sleep#starttimeout=1`,
		exe,
	)

	ts := time.Now()
	prod, err := execHandle(source)
	elapsed := time.Since(ts)

	require.Nil(t, prod)
	require.EqualError(t, err, "exec: timeout")
	require.GreaterOrEqual(t, elapsed, time.Second)
	require.Less(t, elapsed, 10*time.Second)
}

func TestPipeProbeError(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	source := fmt.Sprintf(
		`exec:"%s" -test.run=^TestPipeHelperProcess$ -- garbage#starttimeout=10`,
		exe,
	)

	prod, err := execHandle(source)

	require.Nil(t, prod)
	require.ErrorContains(t, err, "exec/pipe:")
}

// TestPipeHelperProcess acts as a cross-platform exec source.
func TestPipeHelperProcess(t *testing.T) {
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--" {
		return
	}

	switch args[len(args)-1] {
	case "sleep":
		time.Sleep(time.Minute)
	case "garbage":
		_, _ = os.Stdout.WriteString("garbage-not-a-stream")
	default:
		os.Exit(1)
	}

	os.Exit(0)
}
