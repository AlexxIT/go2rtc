package nest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExtendDelay(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	// A normal 5-minute session is extended a minute before it expires.
	require.Equal(t, 4*time.Minute, extendDelay(now.Add(5*time.Minute), now))

	// A missing or near expiresAt must not make the loop extend with no wait.
	require.Equal(t, 30*time.Second, extendDelay(time.Time{}, now))
	require.Equal(t, 30*time.Second, extendDelay(now.Add(45*time.Second), now))

	// Exactly at the floor.
	require.Equal(t, 30*time.Second, extendDelay(now.Add(90*time.Second), now))
	require.Equal(t, 31*time.Second, extendDelay(now.Add(91*time.Second), now))
}
