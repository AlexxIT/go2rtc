package exec

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"io"
	"os/exec"
)

// PipeCloser - return StdoutPipe that Kill cmd on Close call
func PipeCloser(cmd *exec.Cmd) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	return pipeCloser{stdout, cmd}, nil
}

type pipeCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (p pipeCloser) Close() error {
	return core.Any(p.ReadCloser.Close(), p.cmd.Process.Kill(), p.cmd.Wait())
}
