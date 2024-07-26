package mjpeg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRFC2435(t *testing.T) {
	lqt, cqt := MakeTables(71)
	require.Equal(t, byte(9), lqt[0])
	require.Equal(t, byte(10), cqt[0])
}
