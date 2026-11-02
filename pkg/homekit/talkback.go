package homekit

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// FFmpegBin returns the FFmpeg binary used for talkback transcoding.
// internal/homekit points it to the configured ffmpeg.bin.
var FFmpegBin = func() string { return "ffmpeg" }

// talkback converts Opus from the Home app (HomeKit only offers Opus for
// two-way audio) into G.711 for cameras whose backchannel doesn't accept Opus.
// Opus packets go to FFmpeg as RTP over loopback UDP, and raw G.711 comes
// back on stdout in 20 ms frames.
type talkback struct {
	input    *core.Receiver // Opus from HomeKit
	output   *core.Receiver // G.711 for the camera
	sender   *core.Sender
	conn     net.Conn
	cmd      *exec.Cmd
	stopOnce sync.Once
}

func newTalkback(media *core.Media, dst *core.Codec) (*talkback, error) {
	var encoder, format string
	switch dst.Name {
	case core.CodecPCMA:
		encoder, format = "pcm_alaw", "alaw"
	case core.CodecPCMU:
		encoder, format = "pcm_mulaw", "mulaw"
	default:
		return nil, errors.New("homekit: unsupported talkback codec: " + dst.Name)
	}

	clockRate := dst.ClockRate
	if clockRate == 0 {
		clockRate = 8000
	}

	// reserve a free port for FFmpeg, it binds only after reading the SDP
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()

	conn, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(FFmpegBin(),
		"-hide_banner", "-v", "error",
		"-protocol_whitelist", "pipe,udp,rtp",
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-f", "sdp", "-i", "pipe:0",
		"-c:a", encoder, "-ar", strconv.Itoa(int(clockRate)), "-ac", "1",
		"-f", format, "pipe:1",
	)
	cmd.Stdin = newSDP(port)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	t := &talkback{
		input:  core.NewReceiver(media, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}),
		output: core.NewReceiver(media, &core.Codec{Name: dst.Name, ClockRate: clockRate, Channels: 1, PayloadType: dst.PayloadType}),
		conn:   conn,
		cmd:    cmd,
	}

	t.sender = core.NewSender(media, t.input.Codec)
	t.sender.Handler = func(packet *rtp.Packet) {
		// HAP uses a 16 kHz RTP clock for Opus (see opus.RepackToHAP),
		// RFC 7587 (and FFmpeg) expect 48 kHz
		clone := *packet
		clone.Timestamp *= 3
		clone.PayloadType = 110
		if b, err := clone.Marshal(); err == nil {
			_, _ = conn.Write(b)
		}
	}
	t.sender.HandleRTP(t.input)

	go t.readG711(stdout, clockRate/50) // 20 ms frames, 1 byte per sample

	return t, nil
}

func newSDP(port int) io.Reader {
	return &stringReader{s: fmt.Sprintf("v=0\r\n"+
		"o=- 0 0 IN IP4 127.0.0.1\r\n"+
		"s=go2rtc\r\n"+
		"c=IN IP4 127.0.0.1\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP 110\r\n"+
		"a=rtpmap:110 opus/48000/2\r\n", port)}
}

type stringReader struct {
	s string
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

func (t *talkback) readG711(rd io.Reader, frameSize uint32) {
	var seq uint16
	var ts uint32

	for {
		payload := make([]byte, frameSize)
		if _, err := io.ReadFull(rd, payload); err != nil {
			return
		}

		t.output.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    t.output.Codec.PayloadType,
				SequenceNumber: seq,
				Timestamp:      ts,
			},
			Payload: payload,
		})

		seq++
		ts += frameSize
	}
}

func (t *talkback) Stop() {
	t.stopOnce.Do(func() {
		t.sender.Close()
		_ = t.conn.Close()
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		_ = t.cmd.Wait()
	})
}
