package rtsp

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/require"
)

// readRTCP reads one interleaved frame and returns its channel and RTCP packet
func readRTCP(t *testing.T, r io.Reader) (byte, rtcp.Packet) {
	head := make([]byte, 4)
	_, err := io.ReadFull(r, head)
	require.NoError(t, err)
	require.Equal(t, byte('$'), head[0])
	b := make([]byte, int(head[2])<<8|int(head[3]))
	_, err = io.ReadFull(r, b)
	require.NoError(t, err)
	pkts, err := rtcp.Unmarshal(b)
	require.NoError(t, err)
	require.Len(t, pkts, 1)
	return head[1], pkts[0]
}

func TestRequestKeyframeRTCP(t *testing.T) {
	for _, mode := range []string{KeyframePLI, KeyframeFIR} {
		t.Run(mode, func(t *testing.T) {
			client, camera := net.Pipe()
			defer client.Close()
			defer camera.Close()
			r := bufio.NewReader(camera)

			conn := &Conn{Keyframe: mode, conn: client}
			conn.keyframeChan.Store(2)

			// no RTP yet: the SSRC is unknown, nothing is sent
			conn.RequestKeyframe()
			_ = camera.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, err := r.ReadByte()
			require.Error(t, err)
			_ = camera.SetReadDeadline(time.Time{})

			conn.keyframeSSRC.Store(0x1234)
			conn.keyframeNext.Store(0)
			go conn.RequestKeyframe()
			channel, pkt := readRTCP(t, r)
			require.Equal(t, byte(3), channel) // RTCP channel of the track

			switch p := pkt.(type) {
			case *rtcp.PictureLossIndication:
				require.Equal(t, KeyframePLI, mode)
				require.Equal(t, uint32(0x1234), p.MediaSSRC)
			case *rtcp.FullIntraRequest:
				require.Equal(t, KeyframeFIR, mode)
				require.Zero(t, p.MediaSSRC) // RFC 5104: the target is only in the FCI
				require.Equal(t, uint32(0x1234), p.FIR[0].SSRC)
				require.Equal(t, uint8(1), p.FIR[0].SequenceNumber)
			default:
				t.Fatalf("unexpected packet %T", pkt)
			}

			// a request right after is throttled
			conn.RequestKeyframe()
			_ = camera.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, err = r.ReadByte()
			require.Error(t, err)
			_ = camera.SetReadDeadline(time.Time{})

			// the next one after the interval gets a new FIR sequence number
			conn.keyframeNext.Store(0)
			go conn.RequestKeyframe()
			_, pkt = readRTCP(t, r)
			if fir, ok := pkt.(*rtcp.FullIntraRequest); ok {
				require.Equal(t, uint8(2), fir.FIR[0].SequenceNumber)
			}
		})
	}
}

// UDP without a server RTCP port: nothing is sent, and nothing panics
func TestRequestKeyframeRTCPNoUDPPort(t *testing.T) {
	conn := &Conn{Keyframe: KeyframePLI, Transport: "udp"}
	conn.keyframeSSRC.Store(0x1234)
	require.NoError(t, conn.requestKeyframeRTCP())
	require.Zero(t, conn.keyframeNext.Load())
}
