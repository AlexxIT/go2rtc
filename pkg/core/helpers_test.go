package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetRAMUsage(t *testing.T) {
	vMemStat, err := GetRAMUsage()
	require.NoError(t, err)
	require.NotNil(t, vMemStat)
}

func TestGetCPUUsage(t *testing.T) {
	// Short interval to speed up tests; adjust based on your needs
	interval := 100 * time.Millisecond

	avgCPUUsage, err := GetCPUUsage(interval)
	require.NoError(t, err, "GetCPUUsage should not return an error")
	require.GreaterOrEqual(t, avgCPUUsage, 0.0, "Average CPU usage should be >= 0%")
	require.LessOrEqual(t, avgCPUUsage, 100.0, "Average CPU usage should be <= 100%")
}
