package unifiprotect

import (
	"bytes"
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestProducerPrefersOpus(t *testing.T) {
	sps := []byte{0x67, 0x4d, 0x40, 0x32, 0xa6, 0x80, 0x2a}
	pps := []byte{0x68, 0xea, 0x8f, 0x20}
	videoConfig := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	videoFrame := []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 2, 0x65, 0x88}
	opusPacket := append([]byte{0xb8}, bytes.Repeat([]byte{0x42}, 80)...)
	aacConfig := aac.EncodeConfig(aac.TypeAACLC, 16000, 1, false)

	wire := extendedWire(
		&Tag{Type: TagVideo, Data: videoConfig},
		&Tag{Type: TagOpus, Data: opusConfig},
		&Tag{Type: TagVideo, Timestamp: 20, Data: videoFrame},
		&Tag{Type: TagOpus, Timestamp: 20, Data: opusPacket},
		&Tag{Type: TagAudio, Data: append([]byte{0xaf, 0}, aacConfig...)},
		&Tag{Type: TagAudio, Timestamp: 20, Data: []byte{0xaf, 1, 1, 2, 3}},
	)

	p, err := Open(io.NopCloser(bytes.NewReader(wire)), AudioOpus)
	require.NoError(t, err)
	require.Len(t, p.Medias, 2)
	require.Equal(t, core.CodecH264, p.Medias[0].Codecs[0].Name)
	require.Equal(t, core.CodecOpus, p.Medias[1].Codecs[0].Name)
	require.Equal(t, uint32(48000), p.Medias[1].Codecs[0].ClockRate)
	require.Equal(t, uint8(2), p.Medias[1].Codecs[0].Channels)

	videoPackets := attach(t, p, p.Medias[0])
	audioPackets := attach(t, p, p.Medias[1])
	require.ErrorIs(t, p.Start(), io.EOF)
	require.Len(t, *videoPackets, 1)
	require.Equal(t, videoFrame[5:], (*videoPackets)[0].Payload)
	require.Len(t, *audioPackets, 1)
	require.Equal(t, opusPacket, (*audioPackets)[0].Payload)
	require.Equal(t, uint32(960), (*audioPackets)[0].Timestamp)
}

func TestProducerAACFallback(t *testing.T) {
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	videoConfig := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	aacConfig := aac.EncodeConfig(aac.TypeAACLC, 16000, 1, false)
	aacPacket := []byte{1, 2, 3, 4}

	wire := extendedWire(
		&Tag{Type: TagVideo, Data: videoConfig},
		&Tag{Type: TagAudio, Data: append([]byte{0xaf, 0}, aacConfig...)},
		&Tag{Type: TagAudio, Timestamp: 20, Data: append([]byte{0xaf, 1}, aacPacket...)},
	)

	p, err := Open(io.NopCloser(bytes.NewReader(wire)), AudioAAC)
	require.NoError(t, err)
	require.Len(t, p.Medias, 2)
	require.Equal(t, core.CodecAAC, p.Medias[1].Codecs[0].Name)

	packets := attach(t, p, p.Medias[1])
	require.ErrorIs(t, p.Start(), io.EOF)
	require.Len(t, *packets, 1)
	require.Equal(t, aacPacket, (*packets)[0].Payload)
	require.Equal(t, uint32(320), (*packets)[0].Timestamp)
}

func TestProducerVideoOnly(t *testing.T) {
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	videoConfig := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	wire := extendedWire(&Tag{Type: TagVideo, Data: videoConfig})

	p, err := Open(io.NopCloser(bytes.NewReader(wire)), AudioNone)
	require.NoError(t, err)
	require.Len(t, p.Medias, 1)
}

func TestProducerRejectsTruncatedAVCConfig(t *testing.T) {
	// This is a valid FLV AVC sequence header whose AVCDecoderConfigurationRecord
	// ends after the first SPS, before the PPS count byte.
	config := []byte{1, 0x42, 0, 0x1f, 0xff, 0xe1, 0, 1, 0x67}
	wire := extendedWire(&Tag{
		Type: TagVideo,
		Data: append([]byte{0x17, 0, 0, 0, 0}, config...),
	})

	var err error
	require.NotPanics(t, func() {
		_, err = Open(io.NopCloser(bytes.NewReader(wire)), AudioNone)
	})
	require.Error(t, err)
}

func TestValidAVCDecoderConfig(t *testing.T) {
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	config := h264.EncodeConfig(sps, pps)
	require.True(t, validAVCDecoderConfig(config))

	for lengthSizeMinusOne := byte(0); lengthSizeMinusOne < 3; lengthSizeMinusOne++ {
		unsupported := append([]byte(nil), config...)
		unsupported[4] = unsupported[4]&^3 | lengthSizeMinusOne
		require.False(t, validAVCDecoderConfig(unsupported))
	}
}

func TestValidAVCCPayload(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "empty"},
		{name: "missing length", data: []byte{0, 0, 1}},
		{name: "truncated NAL", data: []byte{0, 0, 0, 2, 0x65}},
		{name: "oversized length", data: []byte{0xff, 0xff, 0xff, 0xff, 0x65}},
		{name: "zero length NAL", data: []byte{0, 0, 0, 0}},
		{name: "trailing partial bytes", data: []byte{0, 0, 0, 1, 0x65, 0, 0}},
		{name: "single NAL", data: []byte{0, 0, 0, 2, 0x65, 0x88}, want: true},
		{name: "multiple NALs", data: []byte{0, 0, 0, 1, 0x65, 0, 0, 0, 2, 0x41, 0x88}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, validAVCCPayload(tt.data))
		})
	}
}

func TestProducerDropsMalformedAVCCFrame(t *testing.T) {
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	config := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	malformed := []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 2, 0x65}
	p, err := Open(io.NopCloser(bytes.NewReader(extendedWire(
		&Tag{Type: TagVideo, Data: config},
		&Tag{Type: TagVideo, Data: malformed},
	))), AudioNone)
	require.NoError(t, err)

	packets := attach(t, p, p.Medias[0])
	require.ErrorIs(t, p.Start(), io.EOF)
	require.Empty(t, *packets)
}

func TestProducerProbeBufferLimit(t *testing.T) {
	p := &Producer{probeBytes: maxProbeBuffer - 1}
	require.NoError(t, p.bufferProbeTag(&Tag{Data: []byte{1}}))
	require.ErrorContains(t, p.bufferProbeTag(&Tag{Data: []byte{1}}), "probe exceeds")
	require.Len(t, p.pending, 1)
}

func TestProducerStopCallbackOnce(t *testing.T) {
	sps := []byte{0x67, 0x42, 0, 0x1f, 0xe5, 0x88}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	config := append([]byte{0x17, 0, 0, 0, 0}, h264.EncodeConfig(sps, pps)...)
	p, err := Open(io.NopCloser(bytes.NewReader(extendedWire(&Tag{Type: TagVideo, Data: config}))), AudioNone)
	require.NoError(t, err)

	called := 0
	p.SetOnStop(func() { called++ })
	require.NoError(t, p.Stop())
	require.NoError(t, p.Stop())
	require.Equal(t, 1, called)
}

func attach(t *testing.T, p *Producer, media *core.Media) *[]*rtp.Packet {
	t.Helper()
	receiver, err := p.GetTrack(media, media.Codecs[0])
	require.NoError(t, err)
	packets := []*rtp.Packet{}
	receiver.AppendChild(&core.Node{Input: func(packet *rtp.Packet) {
		clone := packet.Clone()
		packets = append(packets, clone)
	}})
	return &packets
}

func extendedWire(tags ...*Tag) []byte {
	wire := append([]byte(nil), flvHeader...)
	for i, tag := range tags {
		wire = append(wire, flv.EncodeTag(tag.Type, tag.Timestamp, tag.Data)...)
		if i != len(tags)-1 {
			wire = append(wire, bytes.Repeat([]byte{0x55}, 16+i*8)...)
		}
	}
	return wire
}
