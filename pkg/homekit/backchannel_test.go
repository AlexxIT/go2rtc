package homekit

import (
	"net"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
	pionsrtp "github.com/pion/srtp/v3"
	"github.com/stretchr/testify/require"
)

// Opus SILK NB, 20 ms, 1 frame (as sent by Logitech Circle View)
var backchannelPayload = []byte{0x08, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}

func TestBackchannelMedia(t *testing.T) {
	require.Nil(t, backchannelMedia([]camera.AudioCodecConfiguration{{CodecType: camera.AudioCodecTypeAACELD}}))

	media := backchannelMedia([]camera.AudioCodecConfiguration{
		{CodecType: camera.AudioCodecTypeAACELD}, {CodecType: camera.AudioCodecTypeOpus},
	})
	require.NotNil(t, media)
	require.Equal(t, core.DirectionSendonly, media.Direction)
	require.Equal(t, []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}}, media.Codecs)
}

func newBackchannelEndpoint(port int, ssrc uint32) *srtp.Endpoint {
	return &srtp.Endpoint{
		Addr:       "127.0.0.1",
		Port:       uint16(port),
		MasterKey:  []byte(core.RandString(16, 0)),
		MasterSalt: []byte(core.RandString(14, 0)),
		SSRC:       ssrc,
	}
}

// testCamera - fake camera audio endpoint, decrypts with the controller (local) key
type testCamera struct {
	t    *testing.T
	conn net.PacketConn
	ctx  *pionsrtp.Context
}

func newTestCamera(t *testing.T) *testCamera {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &testCamera{t: t, conn: conn}
}

func (c *testCamera) port() int {
	return c.conn.LocalAddr().(*net.UDPAddr).Port
}

func (c *testCamera) read() *rtp.Packet {
	buf := make([]byte, 1500)
	require.NoError(c.t, c.conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	n, _, err := c.conn.ReadFrom(buf)
	require.NoError(c.t, err)

	var header rtp.Header
	b, err := c.ctx.DecryptRTP(nil, buf[:n], &header)
	require.NoError(c.t, err)

	var packet rtp.Packet
	require.NoError(c.t, packet.Unmarshal(b))
	return &packet
}

func (c *testCamera) requireNothing() {
	require.NoError(c.t, c.conn.SetReadDeadline(time.Now().Add(300*time.Millisecond)))
	_, _, err := c.conn.ReadFrom(make([]byte, 1500))
	require.Error(c.t, err) // timeout
}

var opusConfig8k = &camera.AudioCodecConfiguration{
	CodecType:   camera.AudioCodecTypeOpus,
	CodecParams: []camera.AudioCodecParameters{{SampleRate: []byte{camera.AudioCodecSampleRate8Khz}}},
}

func newTestTrack() (*core.Media, *core.Receiver) {
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}
	return media, core.NewReceiver(media, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2})
}

func newTestBackchannel(t *testing.T) (*Client, *testCamera) {
	cam := newTestCamera(t)

	server := srtp.NewServer("127.0.0.1:0")
	session := &srtp.Session{
		Local:        newBackchannelEndpoint(0, 1234),
		Remote:       newBackchannelEndpoint(cam.port(), 5678),
		PayloadType:  110,
		RTCPInterval: time.Hour,
	}
	server.AddSession(session)

	var err error
	cam.ctx, err = pionsrtp.CreateContext(session.Local.MasterKey, session.Local.MasterSalt, pionsrtp.ProtectionProfileAes128CmHmacSha1_80)
	require.NoError(t, err)

	client := &Client{srtp: server, audioSession: session}
	client.startBackchannel(opusConfig8k)
	return client, cam
}

func TestBackchannelHandler(t *testing.T) {
	client, cam := newTestBackchannel(t)

	now := time.Unix(1_000_000, 0)
	handler := client.backchannelHandler(func() time.Time { return now })

	send := func(seq uint16, ts uint32) *rtp.Packet {
		handler(&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: seq, Timestamp: ts}, Payload: backchannelPayload})
		now = now.Add(20 * time.Millisecond)
		return cam.read()
	}

	// first talk spurt, then a second one after 2 s of silence, consumer seq and ts restart
	packets := []*rtp.Packet{send(1, 100_000), send(2, 100_960), send(3, 101_920)}
	now = now.Add(2*time.Second - 20*time.Millisecond)
	packets = append(packets, send(1, 5_000), send(2, 5_960))

	seq0 := packets[0].SequenceNumber
	for i, packet := range packets {
		require.Equal(t, uint8(110), packet.PayloadType)
		require.Equal(t, uint32(1234), packet.SSRC)
		require.Equal(t, backchannelPayload, packet.Payload)
		require.Equal(t, seq0+uint16(i), packet.SequenceNumber) // continuous for the whole session
	}

	require.Equal(t, []bool{true, false, false, true, false}, []bool{
		packets[0].Marker, packets[1].Marker, packets[2].Marker, packets[3].Marker, packets[4].Marker,
	})

	// 960 @ 48 kHz => 160 @ 8 kHz, then 2 s of real time since the last packet
	require.Equal(t, []uint32{0, 160, 320, 320 + 2000*8, 320 + 2000*8 + 160}, []uint32{
		packets[0].Timestamp, packets[1].Timestamp, packets[2].Timestamp, packets[3].Timestamp, packets[4].Timestamp,
	})

	// stopped stream: nothing is sent anymore
	require.NoError(t, client.Stop())
	require.Nil(t, client.txSession.Load())
	handler(&rtp.Packet{Header: rtp.Header{Version: 2, Timestamp: 7_000}, Payload: backchannelPayload})
	cam.requireNothing()
}

func TestBackchannelAddTrack(t *testing.T) {
	client, cam := newTestBackchannel(t)

	media, track1 := newTestTrack()
	require.NoError(t, client.AddTrack(media, nil, track1))
	media, track2 := newTestTrack()
	require.NoError(t, client.AddTrack(media, nil, track2))
	require.Len(t, client.Senders, 1) // single sender for all consumers

	track2.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, Timestamp: 1000}, Payload: backchannelPayload})
	require.Equal(t, backchannelPayload, cam.read().Payload)
}

func TestOpusConfig(t *testing.T) {
	codecs := []camera.AudioCodecConfiguration{{CodecType: camera.AudioCodecTypeAACELD}, {CodecType: camera.AudioCodecTypeOpus}}
	require.Equal(t, &codecs[1], opusConfig(codecs))
	require.Nil(t, opusConfig(codecs[:1]))
}

func TestBackchannelSessionNotAdded(t *testing.T) {
	cam := newTestCamera(t)

	// AddSession was never called (or failed): the session must not be used
	session := &srtp.Session{
		Local:  newBackchannelEndpoint(0, 1234),
		Remote: newBackchannelEndpoint(cam.port(), 5678),
	}
	client := &Client{srtp: srtp.NewServer("127.0.0.1:0"), audioSession: session}
	client.startBackchannel(opusConfig8k)
	require.Nil(t, client.txSession.Load())

	media, track := newTestTrack()
	require.NoError(t, client.AddTrack(media, nil, track))
	track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, Timestamp: 1000}, Payload: backchannelPayload})
	cam.requireNothing() // nothing sent and no panic
}

func TestBackchannelNotOpusSession(t *testing.T) {
	client := &Client{}
	client.startBackchannel(&camera.AudioCodecConfiguration{CodecType: camera.AudioCodecTypeAACELD})
	require.Nil(t, client.txSession.Load())
}

func TestBackchannelCodec(t *testing.T) {
	client := &Client{}
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}
	track := core.NewReceiver(media, &core.Codec{Name: core.CodecPCMU, ClockRate: 8000})
	require.Error(t, client.AddTrack(media, nil, track))
}
