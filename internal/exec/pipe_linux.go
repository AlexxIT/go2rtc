package exec

import (
	"bufio"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// PipeCloser - return StdoutPipe that Kill cmd on Close call
func PipeCloser(cmd *exec.Cmd, params *Params) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// add buffer for pipe reader to reduce syscall
	return pipeCloser{bufio.NewReaderSize(stdout, core.BufferSize), stdout, cmd, params}, nil
}

type pipeCloser struct {
	io.Reader
	io.Closer
	cmd    *exec.Cmd
	params *Params
}

func (p pipeCloser) Close() error {
	finished := make(chan bool)

	if p.params.KillSignal != syscall.SIGKILL {
		go func() {
			select {
			case <-time.After(p.params.KillTimeout):
				p.cmd.Process.Kill()
				break
			case <-finished:
				break
			}
		}()
	}
	err := core.Any(p.Closer.Close(), p.cmd.Process.Signal(p.params.KillSignal), p.cmd.Wait())
	finished <- true
	return err
}
