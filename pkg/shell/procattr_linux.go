package shell

import (
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Pdeathsig preserves the existing parent-death signal for the direct child.
// Setpgid creates a new process group whose ID is the child PID. Descendants
// normally inherit this group, so signaling it also reaches shell wrappers
// and the programs they launch (unless those programs leave the group).
var procAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}

// Give a catchable signal a bounded grace period by default. WaitDelay also
// controls os/exec cleanup of inherited pipes. An explicit killtimeout=0
// disables timed escalation.
const defaultWaitDelay = 5 * time.Second

// signalProcess sends the initial signal to the whole group and, when
// configured, escalates to SIGKILL. os/exec only escalates the direct child,
// which is insufficient for a wrapper that has spawned other processes.
func signalProcess(p *os.Process, sig os.Signal, delay time.Duration) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return p.Signal(sig)
	}

	// A negative PID addresses a process group, not a single process.
	if err := syscall.Kill(-p.Pid, s); err != nil {
		// Match the os/exec Cancel contract for a process that is already gone.
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}

	// SIGKILL needs no grace period. For softer signals, wait in the
	// cancellation path itself instead of scheduling another goroutine.
	// prepareWait keeps the leader PID reserved during this delay.
	// This waits the full delay even if all group members exit sooner.
	if s != syscall.SIGKILL && delay > 0 {
		time.Sleep(delay)
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
	}
	return nil
}

// prepareWait coordinates os/exec cancellation with reaping the group leader.
// Reaping first could release the PID, allowing a delayed group signal to
// target an unrelated group that later acquires the same numeric ID.
func (c *Command) prepareWait() func() error {
	if c.Cancel == nil {
		return c.Cmd.Wait
	}

	// The context watcher and the wait function share one cancellation.
	// Do not reap the leader while a delayed group signal is pending.
	cancel := c.Cancel
	var once sync.Once
	var err error = os.ErrProcessDone
	c.Cancel = func() error {
		once.Do(func() { err = cancel() })
		return err
	}

	return func() error {
		// Wait for exit without reaping: the leader PID must remain reserved
		// until all group signals have been sent, even if the shell exits first.
		var info unix.Siginfo
		var waitErr error
		for {
			waitErr = unix.Waitid(unix.P_PID, c.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
			if waitErr != unix.EINTR {
				break
			}
		}

		// If cancellation won, complete it before reaping. If the leader exited
		// on its own, kill leftover group members now: after reaping, Close
		// could no longer safely address the group by the leader's PID.
		once.Do(func() {
			if waitErr != nil {
				// Without WNOWAIT we cannot safely promise group cleanup. Fail
				// closed rather than leave a delayed signal targeting a freed PID.
				_ = c.Process.Kill()
			} else if c.ctx.Err() != nil {
				err = cancel()
			} else {
				_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
			}
		})

		// Let os/exec reap the leader, collect its exit status and close pipes.
		// Its WaitDelay may add further waiting for I/O cleanup.
		err := c.Cmd.Wait()
		if waitErr != nil {
			return waitErr
		}
		return err
	}
}
