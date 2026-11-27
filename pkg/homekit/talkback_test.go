package homekit

import (
	"net"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestTalkbackOpusToPCMA(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}

	// FFmpeg plays the Home app: a 440 Hz tone as Opus RTP
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()

	port := l.LocalAddr().(*net.UDPAddr).Port
	gen := exec.Command("ffmpeg", "-hide_banner", "-v", "error", "-re",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=48000",
		"-ac", "1", "-c:a", "libopus", "-b:a", "24k", "-frame_duration", "20",
		"-f", "rtp", "rtp://127.0.0.1:"+strconv.Itoa(port))
	require.NoError(t, gen.Start())
	defer func() { _ = gen.Process.Kill(); _ = gen.Wait() }()

	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}
	tb, err := newTalkback(media, &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8})
	require.NoError(t, err)
	defer tb.Stop()

	var mu sync.Mutex
	var packets []*rtp.Packet
	sender := core.NewSender(media, tb.output.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		mu.Lock()
		packets = append(packets, packet)
		mu.Unlock()
	}
	sender.HandleRTP(tb.output)

	b := make([]byte, 1500)
	for {
		_ = l.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := l.ReadFrom(b)
		if err != nil {
			break // generator finished
		}
		packet := &rtp.Packet{}
		if packet.Unmarshal(b[:n]) != nil || packet.PayloadType >= 72 && packet.PayloadType <= 79 {
			continue // RTCP
		}
		packet.Timestamp /= 3 // HAP uses a 16 kHz clock
		tb.input.WriteRTP(packet)
	}

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(packets) >= 50 // at least 1 s of 2 s
	}, 3*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var loud int
	for i, packet := range packets {
		require.Equal(t, uint8(8), packet.PayloadType)
		require.Len(t, packet.Payload, 160) // 20 ms at 8 kHz
		require.Equal(t, uint32(i*160), packet.Timestamp)
		for _, sample := range packet.Payload {
			// A-law silence is around 0xD5/0x55
			if sample&0x7F < 0x40 {
				loud++
			}
		}
	}
	require.Greater(t, loud, len(packets)*40, "tone is missing from the output")
}

func TestConsumerBackchannelAfterStop(t *testing.T) {
	conn, _ := net.Pipe()
	c := NewConsumer(conn, srtp.NewServer("127.0.0.1:0"))
	c.audioSession = &srtp.Session{}

	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}
	track, err := c.GetTrack(media, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2})
	require.NoError(t, err)
	require.Equal(t, core.CodecOpus, track.Codec.Name)
	require.Nil(t, c.talkback)

	require.NoError(t, c.Stop())

	// a late packet from the SRTP server must not crash
	c.audioSession.OnReadRTP(&rtp.Packet{Payload: []byte{1, 2, 3}})
}
