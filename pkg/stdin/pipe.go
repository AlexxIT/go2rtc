package stdin

import (
	"errors"
	"io"
	"os/exec"
)

type pipeCloser struct {
	io.Writer
	io.Closer
	cmd *exec.Cmd
}

func PipeCloser(cmd *exec.Cmd) (io.WriteCloser, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	return pipeCloser{stdin, stdin, cmd}, nil
}

func (p pipeCloser) Close() (err error) {
	var killErr error = nil
	process := p.cmd.Process
	if process != nil {
		killErr = process.Kill()
	}
	return errors.Join(p.Closer.Close(), killErr, p.cmd.Wait())
}
