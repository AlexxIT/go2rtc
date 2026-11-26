package flv

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
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

// Some RTMP servers send a tag with zero data size before the codec headers.
// probe and Start read the payload, so readPacket must skip it.
func TestReadPacketSkipsEmptyTag(t *testing.T) {
	for _, tagType := range []byte{TagAudio, TagVideo, TagData} {
		var b []byte
		b = append(b, 0, 0, 0, 0)                            // previous tag size
		b = append(b, tagType, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // empty tag
		b = append(b, 0, 0, 0, 11)                           // previous tag size
		b = append(b, TagVideo, 0, 0, 2, 0x01, 0x02, 0x03, 0x04, 0, 0, 0)
		b = append(b, 0x17, 0x00)

		c := &Producer{rd: core.NewReadBuffer(bytes.NewReader(b))}
		pkt, err := c.readPacket()
		require.NoError(t, err)
		require.Equal(t, uint8(TagVideo), pkt.PayloadType)
		require.Equal(t, uint32(0x04010203), pkt.Timestamp)
		require.Equal(t, []byte{0x17, 0x00}, pkt.Payload)
	}
}
