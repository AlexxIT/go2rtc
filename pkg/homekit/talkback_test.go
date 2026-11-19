package homekit

import (
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// fakeEncoder returns one "access unit" per frame: the frame's first byte,
// repeated as many times as the frame has samples / 100.
type fakeEncoder struct {
	frames  [][]byte
	onFrame func([]byte)
	closed  bool
}

func (e *fakeEncoder) Write(p []byte) (int, error) {
	e.frames = append(e.frames, append([]byte(nil), p...))
	au := make([]byte, len(p)/200)
	for i := range au {
		au[i] = p[0]
	}
	e.onFrame(au)
	return len(p), nil
}

func (e *fakeEncoder) Close() error {
	e.closed = true
	return nil
}

func withFakeEncoder(t *testing.T) *fakeEncoder {
	enc := &fakeEncoder{}
	prev := NewELDEncoder
	NewELDEncoder = func(sampleRate, frameSize int, onFrame func([]byte)) (io.WriteCloser, error) {
		require.Equal(t, 16000, sampleRate)
		require.Equal(t, 480, frameSize)
		enc.onFrame = onFrame
		return enc, nil
	}
	t.Cleanup(func() { NewELDEncoder = prev })
	return enc
}

func TestELDPayload(t *testing.T) {
	au := make([]byte, 100)
	payload := eldPayload(au)
	// 16 bits of AU-headers, then size 100 << 3 = 0x0320 with index 0
	require.Equal(t, []byte{0x00, 0x10, 0x03, 0x20}, payload[:4])
	require.Len(t, payload, 104)
}

func TestTalkbackFraming(t *testing.T) {
	enc := withFakeEncoder(t)

	var packets []*rtp.Packet
	tb, err := newTalkback(16000, 480, func(packet *rtp.Packet) {
		packets = append(packets, packet)
	})
	require.NoError(t, err)

	// 20 ms packets at 16 kHz are 640 bytes, frames are 960 bytes (30 ms)
	for i := byte(1); i <= 6; i++ {
		chunk := make([]byte, 640)
		for j := range chunk {
			chunk[j] = i
		}
		require.NoError(t, tb.WritePCM(chunk))
	}

	// 3840 bytes make exactly four frames, cut across the input packets
	require.Len(t, enc.frames, 4)
	for _, frame := range enc.frames {
		require.Len(t, frame, 960)
	}
	require.Equal(t, byte(2), enc.frames[1][0])
	require.Empty(t, tb.pending)

	require.Len(t, packets, 4)
	for i, packet := range packets {
		require.Equal(t, uint16(i), packet.SequenceNumber)
		require.Equal(t, uint32(i*480), packet.Timestamp)
		require.True(t, packet.Marker)
		// One AU-header for the fake encoder's 4-byte unit, then the unit
		require.Equal(t, []byte{0x00, 0x10, 0x00, 4 << 3}, packet.Payload[:4])
		require.Len(t, packet.Payload, 8)
	}

	require.NoError(t, tb.Close())
	require.True(t, enc.closed)
}

func TestTalkbackMedia(t *testing.T) {
	speaker := &hap.Service{Type: typeSpeaker}
	camera0 := &hap.Service{Type: "110"}
	eld := camera.SupportedAudioStreamConfiguration{
		Codecs: []camera.AudioCodecConfiguration{{CodecType: camera.AudioCodecTypeAACELD}},
	}
	opus := camera.SupportedAudioStreamConfiguration{
		Codecs: []camera.AudioCodecConfiguration{{CodecType: camera.AudioCodecTypeOpus}},
	}
	withSpeaker := &hap.Accessory{Services: []*hap.Service{camera0, speaker}}
	withoutSpeaker := &hap.Accessory{Services: []*hap.Service{camera0}}

	// No encoder configured: no talk-back
	prev := NewELDEncoder
	NewELDEncoder = nil
	require.Nil(t, talkbackMedia(withSpeaker, &eld))
	NewELDEncoder = prev

	withFakeEncoder(t)

	media := talkbackMedia(withSpeaker, &eld)
	require.NotNil(t, media)
	require.Equal(t, core.DirectionSendonly, media.Direction)
	require.Equal(t, core.CodecPCMA, media.Codecs[0].Name)

	require.Nil(t, talkbackMedia(withoutSpeaker, &eld))
	require.Nil(t, talkbackMedia(withSpeaker, &opus))
}
