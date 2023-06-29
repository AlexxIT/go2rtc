package mp4

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStartH264(t *testing.T) {
	codec := &core.Codec{Name: core.CodecH264}
	track := core.NewReceiver(nil, codec)

	packetKey := &rtp.Packet{
		Header:  rtp.Header{Marker: true},
		Payload: []byte{h264.NALUTypeIFrame, 0, 0},
	}

	packetNotKey := &rtp.Packet{
		Header:  rtp.Header{Marker: true},
		Payload: []byte{h264.NALUTypePFrame, 0, 0},
	}

	cons := &Consumer{}
	err := cons.AddTrack(nil, nil, track)
	require.Nil(t, err)

	track.WriteRTP(packetKey)
	time.Sleep(time.Millisecond)

	_, err = cons.Init()
	require.Nil(t, err)

	cons.Start()

	track.WriteRTP(packetNotKey)
	time.Sleep(time.Millisecond)

	require.Zero(t, cons.send)

	track.WriteRTP(packetKey)
	time.Sleep(time.Millisecond)

	require.NotZero(t, cons.send)
}

func TestStartOPUS(t *testing.T) {
	// Test for fix this issue
	// https://github.com/AlexxIT/go2rtc/issues/404
	codec := &core.Codec{Name: core.CodecOpus}
	track := core.NewReceiver(nil, codec)

	cons := &Consumer{}
	err := cons.AddTrack(nil, nil, track)
	require.Nil(t, err)

	track.WriteRTP(&rtp.Packet{
		Payload: []byte{0},
	})
	time.Sleep(time.Millisecond)

	require.Zero(t, cons.send)

	_, err = cons.Init()
	require.Nil(t, err)

	cons.Start()

	track.WriteRTP(&rtp.Packet{
		Payload: []byte{0},
	})
	time.Sleep(time.Millisecond)

	require.NotZero(t, cons.send)
}
