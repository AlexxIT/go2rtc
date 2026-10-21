package flv

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimeToRTP(t *testing.T) {
	// Reolink camera has 20 FPS
	// Video timestamp increases by 50ms, SampleRate 90000, RTP timestamp increases by 4500
	// Audio timestamp increases by 64ms, SampleRate 16000, RTP timestamp increases by 1024
	frameN := 1
	for i := 0; i < 32; i++ {
		// 1000ms/(90000/4500) = 50ms
		require.Equal(t, uint32(frameN*4500), TimeToRTP(uint32(frameN*50), 90000))
		// 1000ms/(16000/1024) = 64ms
		require.Equal(t, uint32(frameN*1024), TimeToRTP(uint32(frameN*64), 16000))
		frameN *= 2
	}
}

func TestProbe(t *testing.T) {
	// Construct a stream that flv.Open would read from Conn:
	// 9 bytes of FLV header
	// 4 bytes PreviousTagSize0 = 0
	// 11 bytes TagHeader: type 9 (video), size 30, time 0, streamID 0
	// 30 bytes payload (H264 sequence header)
	var b []byte
	b = append(b, 'F', 'L', 'V', 1, 0, 0, 0, 0, 9)
	b = append(b, 0, 0, 0, 0) // previous tag size
	b = append(b, 9)          // tag video
	b = append(b, 0, 0, 30)   // size 30
	b = append(b, 0, 0, 0, 0) // time
	b = append(b, 0, 0, 0)    // streamID 0
	avc_seq := []byte{0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0x42, 0x00, 0x1f, 0xff, 0xe1, 0x00, 0x0a, 0x27, 0x42, 0x00, 0x1f, 0x8d, 0x68, 0x05, 0x00, 0x5b, 0xa1, 0x01, 0x00, 0x04, 0x28, 0xce, 0x3c, 0x80}
	b = append(b, avc_seq...)

	prod, err := Open(bytes.NewReader(b))
	require.NoError(t, err)
	require.NotNil(t, prod)
	require.NotEmpty(t, prod.Medias)
	t.Logf("Medias: %+v", prod.Medias[0].Codecs[0])
}

