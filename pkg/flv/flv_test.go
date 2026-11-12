package flv

import (
	"bytes"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/av1"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
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

// av1SeqHdrOBU is a sequence header OBU captured from an ffmpeg av1_nvenc
// 1920x1080 stream, as carried in the av1C configOBUs.
var av1SeqHdrOBU = []byte{
	0x0a, 0x0e, 0x00, 0x00, 0x00, 0x42, 0xab, 0xbf,
	0xc3, 0x70, 0x08, 0x66, 0x40, 0x40, 0x40, 0x41,
}

func flvTag(tagType byte, payload []byte) []byte {
	b := make([]byte, 4+11, 4+11+len(payload))
	b[4] = tagType
	b[5] = byte(len(payload) >> 16)
	b[6] = byte(len(payload) >> 8)
	b[7] = byte(len(payload))
	return append(b, payload...)
}

// TestProducerAV1 feeds an enhanced-RTMP AV1 stream through the producer.
// ffmpeg writes AV1 with fourCC av01, and enhanced-RTMP carries the 24-bit
// composition time offset for HEVC only, so the OBUs of a PacketTypeCodedFrames
// body start 5 bytes in, not 8.
func TestProducerAV1(t *testing.T) {
	b := []byte{'F', 'L', 'V', 1, 5, 0, 0, 0, 9}

	// metadata without audiocodecid, so the probe doesn't wait for audio
	b = append(b, flvTag(TagData, []byte("onMetaDatawidth"))...)

	// PacketTypeSequenceStart, fourCC av01, av1C record
	av1c := append([]byte{0x81, 0x08, 0x0c, 0x00}, av1SeqHdrOBU...)
	b = append(b, flvTag(TagVideo, append([]byte{0x90, 'a', 'v', '0', '1'}, av1c...))...)

	// PacketTypeCodedFrames, fourCC av01, temporal delimiter + frame OBU
	obus := []byte{0x12, 0x00, 0x32, 0x01, 0x20}
	b = append(b, flvTag(TagVideo, append([]byte{0x91, 'a', 'v', '0', '1'}, obus...))...)
	b = append(b, 0, 0, 0, 0)

	prod, err := Open(bytes.NewReader(b))
	require.Nil(t, err)
	require.Len(t, prod.Medias, 1)

	codec := prod.Medias[0].Codecs[0]
	require.Equal(t, core.CodecAV1, codec.Name)
	require.Equal(t, av1SeqHdrOBU, av1.GetSequenceHeader(codec.FmtpLine))

	receiver, err := prod.GetTrack(prod.Medias[0], codec)
	require.Nil(t, err)

	packets := make(chan []byte, 8)
	sender := core.NewSender(prod.Medias[0], codec)
	sender.Handler = func(packet *rtp.Packet) { packets <- packet.Payload }
	sender.HandleRTP(receiver)
	defer sender.Close()

	_ = prod.Start()

	select {
	case payload := <-packets:
		require.Equal(t, obus, payload)
	case <-time.After(time.Second):
		t.Fatal("no video packet")
	}
}

// enhanced-RTMP carries the 24 bit composition time offset only for the codecs
// that reorder frames. Getting this wrong shifts every frame by 3 bytes.
func TestCodedFramesOffset(t *testing.T) {
	for _, tt := range []struct {
		fourCC string
		want   int
	}{
		{FourCCAV1, 5},
		{"vp09", 5},
		{FourCCAVC, 8},
		{FourCCHEVC, 8},
		{FourCCVVC, 8},
	} {
		require.Equal(t, tt.want, codedFramesOffset(tt.fourCC), "codedFramesOffset(%q)", tt.fourCC)
	}
}
