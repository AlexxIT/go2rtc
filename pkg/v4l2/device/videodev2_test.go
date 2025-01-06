package device

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestSize(t *testing.T) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		require.Equal(t, 104, int(unsafe.Sizeof(v4l2_capability{})))
		require.Equal(t, 208, int(unsafe.Sizeof(v4l2_format{})))
		require.Equal(t, 204, int(unsafe.Sizeof(v4l2_streamparm{})))
		require.Equal(t, 20, int(unsafe.Sizeof(v4l2_requestbuffers{})))
		require.Equal(t, 88, int(unsafe.Sizeof(v4l2_buffer{})))
		require.Equal(t, 16, int(unsafe.Sizeof(v4l2_timecode{})))
		require.Equal(t, 64, int(unsafe.Sizeof(v4l2_fmtdesc{})))
		require.Equal(t, 44, int(unsafe.Sizeof(v4l2_frmsizeenum{})))
		require.Equal(t, 52, int(unsafe.Sizeof(v4l2_frmivalenum{})))
	case "386", "arm":
		require.Equal(t, 104, int(unsafe.Sizeof(v4l2_capability{})))
		require.Equal(t, 204, int(unsafe.Sizeof(v4l2_format{})))
		require.Equal(t, 204, int(unsafe.Sizeof(v4l2_streamparm{})))
		require.Equal(t, 20, int(unsafe.Sizeof(v4l2_requestbuffers{})))
		require.Equal(t, 68, int(unsafe.Sizeof(v4l2_buffer{})))
		require.Equal(t, 16, int(unsafe.Sizeof(v4l2_timecode{})))
		require.Equal(t, 64, int(unsafe.Sizeof(v4l2_fmtdesc{})))
		require.Equal(t, 44, int(unsafe.Sizeof(v4l2_frmsizeenum{})))
		require.Equal(t, 52, int(unsafe.Sizeof(v4l2_frmivalenum{})))
	}
}
