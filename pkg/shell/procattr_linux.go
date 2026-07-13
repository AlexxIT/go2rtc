package shell

import (
	"errors"
	"os/exec"
	"syscall"
)

// will stop child if parent died (even with SIGKILL)
var procAttr = &syscall.SysProcAttr{
	Pdeathsig: syscall.SIGTERM,
	Setpgid:   true,
}

func terminateCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		if err = syscall.Kill(-pgid, syscall.SIGTERM); err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}

	return nil
}

func killCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		if err = syscall.Kill(-pgid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
	}

	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}

	return nil
}
