package exec

import (
	"bufio"
	"errors"
	"io"
	"net/url"
	"os/exec"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// PipeCloser - return StdoutPipe that Kill cmd on Close call
func PipeCloser(cmd *exec.Cmd, query url.Values) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// add buffer for pipe reader to reduce syscall
	return &pipeCloser{bufio.NewReaderSize(stdout, core.BufferSize), stdout, cmd, query}, nil
}

type pipeCloser struct {
	io.Reader
	io.Closer
	cmd   *exec.Cmd
	query url.Values
}

func (p *pipeCloser) Close() error {
	return errors.Join(p.Closer.Close(), p.Kill(), p.Wait())
}

func (p *pipeCloser) Kill() error {
	if s := p.query.Get("killsignal"); s != "" {
		log.Trace().Msgf("[exec] kill with custom sig=%s", s)
		sig := syscall.Signal(core.Atoi(s))
		return p.cmd.Process.Signal(sig)
	}
	return p.cmd.Process.Kill()
}

func (p *pipeCloser) Wait() error {
	if s := p.query.Get("killtimeout"); s != "" {
		timeout := time.Duration(core.Atoi(s)) * time.Second
		timer := time.AfterFunc(timeout, func() {
			log.Trace().Msgf("[exec] kill after timeout=%s", s)
			_ = p.cmd.Process.Kill()
		})
		defer timer.Stop() // stop timer if Wait ends before timeout
	}
	return p.cmd.Wait()
}
