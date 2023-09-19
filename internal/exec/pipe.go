package exec

import (
	"bufio"
	"io"
	"os/exec"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// PipeCloser - return StdoutPipe that Kill cmd on Close call
func PipeCloser(cmd *exec.Cmd) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// add buffer for pipe reader to reduce syscall
	return pipeCloser{bufio.NewReaderSize(stdout, core.BufferSize), stdout, cmd}, nil
}

type pipeCloser struct {
	io.Reader
	io.Closer
	cmd *exec.Cmd
}

func (p pipeCloser) Close() error {
	return core.Any(p.Closer.Close(), p.cmd.Process.Kill(), p.cmd.Wait())
}
