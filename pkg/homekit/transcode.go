package homekit

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

var ffmpegBin = "ffmpeg"

func SetFFmpegBin(bin string) {
	if bin != "" {
		ffmpegBin = bin
	}
}

type audioTranscoder struct {
	input    net.Conn
	cmd      *exec.Cmd
	sender   *core.Sender
	once     sync.Once
	waitOnce sync.Once
	waitErr  error

	receiver *core.Receiver
}

func newAudioTranscoder(src *core.Receiver, dst *core.Codec) (*audioTranscoder, error) {
	rtpIn, err := rtpInput(src.Codec)
	if err != nil {
		return nil, err
	}
	output, err := rawAudioOutput(dst)
	if err != nil {
		return nil, fmt.Errorf("homekit: unsupported talkback transcode: %s to %s: %w", src.Codec.Name, dst.Name, err)
	}

	reservation, err := reserveUDPPort()
	if err != nil {
		return nil, err
	}
	inPort := reservation.LocalAddr().(*net.UDPAddr).Port

	args := []string{
		"-hide_banner", "-v", "error",
		"-protocol_whitelist", "pipe,udp,rtp",
		"-f", "sdp", "-i", "pipe:0",
		"-map", "0:a:0",
		"-c:a", output.encoder,
		"-ar:a", strconv.Itoa(int(output.clockRate)),
		"-ac:a", strconv.Itoa(int(output.channels)),
		"-f", output.muxer, "pipe:1",
	}
	cmd := exec.Command(ffmpegBin, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = reservation.Close()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		_ = reservation.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = reservation.Close()
		return nil, err
	}

	payloadType := src.Codec.PayloadType
	if payloadType == 0 || payloadType == core.PayloadTypeRAW {
		payloadType = 110
	}
	sdp := fmt.Sprintf("v=0\r\n"+
		"o=- 0 0 IN IP4 127.0.0.1\r\n"+
		"s=go2rtc-homekit-talkback\r\n"+
		"c=IN IP4 127.0.0.1\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP %d\r\n"+
		"a=rtpmap:%d %s\r\n",
		inPort, payloadType, payloadType, rtpIn.rtpmap)
	if rtpIn.fmtp != "" {
		sdp += fmt.Sprintf("a=fmtp:%d %s\r\n", payloadType, rtpIn.fmtp)
	}

	udpInput, err := net.Dial("udp", "127.0.0.1:"+strconv.Itoa(inPort))
	if err != nil {
		_ = stdin.Close()
		_ = reservation.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	_ = reservation.Close()
	if _, err = stdin.Write([]byte(sdp)); err != nil {
		_ = stdin.Close()
		_ = udpInput.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	_ = stdin.Close()

	t := &audioTranscoder{
		input:    udpInput,
		cmd:      cmd,
		receiver: core.NewReceiver(src.Media, output.codec),
	}

	go output.read(t, stdout)
	go func() {
		_ = t.wait()
		t.Close()
	}()

	sender := core.NewSender(src.Media, src.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if b, err := packet.Marshal(); err == nil {
			_, _ = udpInput.Write(b)
		}
	}
	sender.HandleRTP(src)
	t.sender = sender

	return t, nil
}

func (t *audioTranscoder) Receiver() *core.Receiver {
	return t.receiver
}

func (t *audioTranscoder) Close() error {
	t.once.Do(func() {
		if t.sender != nil {
			t.sender.Close()
		}
		if t.receiver != nil {
			t.receiver.Close()
		}
		if t.input != nil {
			_ = t.input.Close()
		}
		if t.cmd != nil && t.cmd.Process != nil && t.cmd.ProcessState == nil {
			_ = t.cmd.Process.Kill()
		}
		_ = t.wait()
	})
	return nil
}

func (t *audioTranscoder) wait() error {
	t.waitOnce.Do(func() {
		if t.cmd != nil {
			t.waitErr = t.cmd.Wait()
		}
	})
	return t.waitErr
}

type audioRTPInput struct {
	rtpmap string
	fmtp   string
}

func rtpInput(codec *core.Codec) (*audioRTPInput, error) {
	clockRate := codec.ClockRate
	if clockRate == 0 {
		clockRate = 48000
	}
	channels := codec.Channels
	if channels == 0 {
		channels = 1
	}

	var name string
	switch codec.Name {
	case core.CodecOpus:
		name = "opus"
		clockRate = 48000
		channels = 2
	case core.CodecELD:
		name = "MPEG4-GENERIC"
	case core.CodecPCMA:
		name = "PCMA"
	case core.CodecPCMU:
		name = "PCMU"
	default:
		return nil, fmt.Errorf("homekit: unsupported talkback input codec: %s", codec.Name)
	}

	return &audioRTPInput{
		rtpmap: fmt.Sprintf("%s/%d/%d", name, clockRate, channels),
		fmtp:   codec.FmtpLine,
	}, nil
}

type audioRawOutput struct {
	codec     *core.Codec
	encoder   string
	muxer     string
	clockRate uint32
	channels  uint8
	read      func(*audioTranscoder, io.Reader)
}

func rawAudioOutput(codec *core.Codec) (*audioRawOutput, error) {
	output := &audioRawOutput{
		codec:     codec.Clone(),
		clockRate: codec.ClockRate,
		channels:  codec.Channels,
	}
	if output.clockRate == 0 {
		output.clockRate = 8000
	}
	if output.channels == 0 {
		output.channels = 1
	}

	switch codec.Name {
	case core.CodecPCMA:
		output.encoder = "pcm_alaw"
		output.muxer = "alaw"
		output.read = output.readFixedFrames(1, 20)
	case core.CodecPCMU:
		output.encoder = "pcm_mulaw"
		output.muxer = "mulaw"
		output.read = output.readFixedFrames(1, 20)
	case core.CodecPCM:
		output.encoder = "pcm_s16be"
		output.muxer = "s16be"
		output.read = output.readFixedFrames(2, 20)
	case core.CodecPCML:
		output.encoder = "pcm_s16le"
		output.muxer = "s16le"
		output.read = output.readFixedFrames(2, 20)
	case core.CodecAAC:
		if output.clockRate == 8000 {
			output.clockRate = 16000
		}
		output.encoder = "aac"
		output.muxer = "adts"
		output.codec.PayloadType = core.PayloadTypeRAW
		output.read = output.readADTS
	default:
		return nil, fmt.Errorf("unsupported output codec")
	}

	return output, nil
}

func (o *audioRawOutput) readFixedFrames(bytesPerSample, frameMs int) func(*audioTranscoder, io.Reader) {
	samplesPerFrame := int(o.clockRate) * frameMs / 1000
	payloadSize := samplesPerFrame * int(o.channels) * bytesPerSample

	return func(t *audioTranscoder, rd io.Reader) {
		buf := make([]byte, payloadSize)

		var seq uint16
		var timestamp uint32

		for {
			if _, err := io.ReadFull(rd, buf); err != nil {
				return
			}

			payload := make([]byte, payloadSize)
			copy(payload, buf)

			t.receiver.WriteRTP(&rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    o.codec.PayloadType,
					SequenceNumber: seq,
					Timestamp:      timestamp,
				},
				Payload: payload,
			})

			seq++
			timestamp += uint32(samplesPerFrame)
		}
	}
}

func (o *audioRawOutput) readADTS(t *audioTranscoder, rd io.Reader) {
	var seq uint16
	var timestamp uint32

	for {
		header := make([]byte, aac.ADTSHeaderSize)
		if _, err := io.ReadFull(rd, header); err != nil {
			return
		}
		if !aac.IsADTS(header) {
			return
		}

		size := int(aac.ReadADTSSize(header))
		if size < aac.ADTSHeaderSize {
			return
		}
		payload := make([]byte, size-aac.ADTSHeaderSize)
		if _, err := io.ReadFull(rd, payload); err != nil {
			return
		}

		t.receiver.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    o.codec.PayloadType,
				SequenceNumber: seq,
				Timestamp:      timestamp,
			},
			Payload: payload,
		})

		seq++
		timestamp += aac.AUTime
	}
}

func reserveUDPPort() (net.PacketConn, error) {
	return net.ListenPacket("udp", "127.0.0.1:0")
}
