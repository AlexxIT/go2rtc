package homekit

import (
	"bufio"
	"encoding/binary"
	"io"
	"os/exec"
	"strconv"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
)

// initTalkback enables talk-back to HomeKit cameras when an AAC-ELD encoder is
// configured:
//
//	homekit_client:
//	  aac_eld_encoder: /usr/local/bin/eld-enc
//
// The encoder is a separate program because go2rtc can't include one. It reads
// mono 16-bit little-endian PCM on stdin and writes each encoded access unit to
// stdout, prefixed with its 2-byte big-endian length. It is started as
// "<encoder> -r <sample rate> -f <frame size>".
func initTalkback() {
	var cfg struct {
		Mod struct {
			Encoder string `yaml:"aac_eld_encoder"`
		} `yaml:"homekit_client"`
	}
	app.LoadConfig(&cfg)

	if cfg.Mod.Encoder == "" {
		return
	}

	homekit.NewELDEncoder = func(sampleRate, frameSize int, onFrame func([]byte)) (io.WriteCloser, error) {
		return startEncoder(cfg.Mod.Encoder, sampleRate, frameSize, onFrame)
	}
}

type encoder struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func startEncoder(path string, sampleRate, frameSize int, onFrame func([]byte)) (*encoder, error) {
	cmd := exec.Command(path, "-r", strconv.Itoa(sampleRate), "-f", strconv.Itoa(frameSize))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}

	log.Debug().Msgf("[homekit] talk-back encoder started: %s", cmd)

	go func() {
		rd := bufio.NewReader(stdout)
		var size [2]byte
		for {
			if _, err := io.ReadFull(rd, size[:]); err != nil {
				break
			}
			au := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(rd, au); err != nil {
				break
			}
			onFrame(au)
		}
		if err := cmd.Wait(); err != nil {
			log.Warn().Err(err).Msg("[homekit] talk-back encoder")
		}
	}()

	return &encoder{cmd: cmd, stdin: stdin}, nil
}

func (e *encoder) Write(p []byte) (int, error) {
	return e.stdin.Write(p)
}

// Close ends the input; the encoder then flushes and exits.
func (e *encoder) Close() error {
	return e.stdin.Close()
}
