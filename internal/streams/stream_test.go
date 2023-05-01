package streams

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTemplate(t *testing.T) {
	source1 := "does not matter"

	stream1 := New("from_yaml", source1)
	require.Len(t, streams, 1)

	stream2 := NewTemplate("camera.from_hass", "rtsp://localhost:8554/from_yaml?video")

	require.Equal(t, stream1, stream2)
	require.Equal(t, stream2.producers[0].url, source1)
	require.Len(t, streams, 2)
}
