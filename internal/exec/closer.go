package exec

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// closer support custom killsignal with custom killtimeout
type closer struct {
	cmd   *exec.Cmd
	query url.Values
}

func (c *closer) Close() (err error) {
	sig := os.Kill
	if s := c.query.Get("killsignal"); s != "" {
		sig = syscall.Signal(core.Atoi(s))
	}

	log.Trace().Msgf("[exec] kill with signal=%d", sig)
	err = c.cmd.Process.Signal(sig)

	if s := c.query.Get("killtimeout"); s != "" {
		timeout := time.Duration(core.Atoi(s)) * time.Second
		timer := time.AfterFunc(timeout, func() {
			log.Trace().Msgf("[exec] kill after timeout=%s", s)
			_ = c.cmd.Process.Kill()
		})
		defer timer.Stop() // stop timer if Wait ends before timeout
	}

	return errors.Join(err, c.cmd.Wait())
}
