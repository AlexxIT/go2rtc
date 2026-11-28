//go:build !linux

package shell

import (
	"os"
	"syscall"
	"time"
)

// Keep the existing process attributes on non-Linux platforms.
var procAttr *syscall.SysProcAttr

// Do not change the existing default wait behavior outside Linux.
const defaultWaitDelay = 0

// Fall back to signaling the direct child. Any configured WaitDelay
// is handled by os/exec itself.
func signalProcess(p *os.Process, sig os.Signal, _ time.Duration) error {
	return p.Signal(sig)
}

// No delayed group signal means no special coordination with reaping.
func (c *Command) prepareWait() func() error {
	return c.Cmd.Wait
}
