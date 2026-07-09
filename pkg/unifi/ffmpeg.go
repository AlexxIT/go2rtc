package unifi

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

type ffmpegCommand interface {
	StdinPipe() (io.WriteCloser, error)
	Start() error
	Wait() error
	Close() error
}

type ffmpegStderr interface {
	StderrPipe() (io.ReadCloser, error)
}

type rtpWriter interface {
	io.WriteCloser
	SetWriteDeadline(time.Time) error
}

type talkbackOutput struct {
	cmd      ffmpegCommand
	rtp      rtpWriter
	closeErr error
	close    sync.Once
}

var newFFmpegCommand = func(command string) ffmpegCommand {
	return shell.NewCommand(command)
}

var reserveRTPPort = func() (int, error) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.Port, nil
}

var dialRTPPort = func(port int) (rtpWriter, error) {
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	return &udpRTPWriter{UDPConn: conn, addr: addr}, nil
}

type udpRTPWriter struct {
	*net.UDPConn
	addr *net.UDPAddr
}

func (w *udpRTPWriter) Write(b []byte) (int, error) {
	return w.WriteTo(b, w.addr)
}

func startFFmpegRTP(inputCodec *core.Codec, session *TalkbackSession) (*talkbackOutput, error) {
	command, err := buildFFmpegCommand(session.URL, session.Codec, session.SamplingRate)
	if err != nil {
		return nil, err
	}

	port, err := reserveRTPPort()
	if err != nil {
		return nil, err
	}

	log.Debug().
		Int("local_port", port).
		Str("codec", session.Codec).
		Int("sampling_rate", session.SamplingRate).
		Str("url", creds.SecretString(session.URL)).
		Msg("[unifi] start ffmpeg")
	log.Trace().
		Str("cmd", creds.SecretString(command)).
		Msg("[unifi] ffmpeg command")

	cmd := newFFmpegCommand(command)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	var stderr io.ReadCloser
	if cmd, ok := cmd.(ffmpegStderr); ok {
		stderr, _ = cmd.StderrPipe()
	}

	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	if stderr != nil {
		go logFFmpegStderr(stderr)
	}

	if _, err = io.WriteString(stdin, buildInputSDP(inputCodec, port)); err != nil {
		_ = stdin.Close()
		_ = cmd.Close()
		return nil, err
	}
	_ = stdin.Close()

	rtp, err := dialRTPPort(port)
	if err != nil {
		_ = cmd.Close()
		return nil, err
	}

	return &talkbackOutput{cmd: cmd, rtp: rtp}, nil
}

func logFFmpegStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		log.Debug().
			Str("line", creds.SecretString(scanner.Text())).
			Msg("[unifi] ffmpeg")
	}
}

func buildFFmpegCommand(outputURL, codec string, samplingRate int) (string, error) {
	audioCodec, err := ffmpegAudioCodec(codec)
	if err != nil {
		return "", err
	}

	args := []string{
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-protocol_whitelist", "file,pipe,udp,rtp",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-f", "sdp",
		"-i", "pipe:0",
		"-map", "0:a:0",
		"-vn",
		"-c:a", audioCodec,
		"-application:a", "lowdelay",
		"-ar:a", strconv.Itoa(samplingRate),
		"-ac:a", "1",
		"-flush_packets", "1",
		"-f", "rtp",
		outputURL,
	}

	return strings.Join(args, " "), nil
}

func ffmpegAudioCodec(codec string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "opus":
		return "libopus", nil
	default:
		return "", fmt.Errorf("unifi: unsupported talkback codec: %s", codec)
	}
}

func buildInputSDP(codec *core.Codec, port int) string {
	payloadType, rtpmap := inputRTPMap(codec)

	return fmt.Sprintf(
		"v=0\r\n"+
			"o=- 0 0 IN IP4 127.0.0.1\r\n"+
			"s=go2rtc-unifi-talkback\r\n"+
			"c=IN IP4 127.0.0.1\r\n"+
			"t=0 0\r\n"+
			"m=audio %d RTP/AVP %d\r\n"+
			"a=rtpmap:%d %s\r\n"+
			"a=recvonly\r\n",
		port, payloadType, payloadType, rtpmap,
	)
}

func inputRTPMap(codec *core.Codec) (uint8, string) {
	switch codec.Name {
	case core.CodecPCMU:
		return 0, "PCMU/8000"
	case core.CodecPCMA:
		return 8, "PCMA/8000"
	case core.CodecOpus:
		return 111, "opus/48000/2"
	}

	payloadType := codec.PayloadType
	if payloadType == 0 {
		payloadType = 111
	}

	clockRate := codec.ClockRate
	if clockRate == 0 {
		clockRate = 48000
	}

	codecName := strings.ToLower(codec.Name)
	if codecName == "" {
		codecName = "opus"
	}

	rtpmap := fmt.Sprintf("%s/%d", codecName, clockRate)
	if codec.Channels > 1 {
		rtpmap += fmt.Sprintf("/%d", codec.Channels)
	}

	return payloadType, rtpmap
}

func (o *talkbackOutput) Close() error {
	o.close.Do(func() {
		log.Debug().Msg("[unifi] stop ffmpeg")

		if o.rtp != nil {
			o.closeErr = o.rtp.Close()
		}
		if o.cmd != nil {
			if closeErr := o.cmd.Close(); o.closeErr == nil {
				o.closeErr = closeErr
			}
		}
	})
	return o.closeErr
}
