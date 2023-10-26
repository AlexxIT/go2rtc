package expr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchHost(t *testing.T) {
	v, err := Run(`
let url = "rtsp://user:pass@192.168.1.123/cam/realmonitor?...";
let host = match(url, "//[^/]+")[0][2:];
host
`)
	require.Nil(t, err)
	require.Equal(t, "user:pass@192.168.1.123", v)
}
