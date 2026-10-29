//go:build linux

package exec

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func init() {
	log = zerolog.Nop() // package log is normally set by Init()
}

// TestPipeProbeTimeout: an exec pipe source that never produces output must
// fail with a timeout instead of blocking GetProducer forever. magic.Open
// used to block indefinitely on a child that never writes a byte.
func TestPipeProbeTimeout(t *testing.T) {
	ts := time.Now()

	prod, err := execHandle("exec:sleep 60#starttimeout=2")

	require.Nil(t, prod)
	require.ErrorContains(t, err, "timeout")
	require.Less(t, time.Since(ts), 10*time.Second)
	require.GreaterOrEqual(t, time.Since(ts), 2*time.Second)
}

// TestPipeProbeError: a child whose output matches no known magic must fail
// fast with the probe error, not with the timeout. This proves the probe
// result path is wired up.
func TestPipeProbeError(t *testing.T) {
	_, err := execHandle("exec:echo garbage-not-a-stream#starttimeout=10")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "exec: timeout")
}
