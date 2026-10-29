package shell

import (
	"context"
	"os/exec"
	"time"
)

// killGrace is how long a child gets to exit after Close() fires the kill
// signal, before the whole process group is sent SIGKILL. It is also the
// default WaitDelay, so Wait() cannot hang forever on pipes held open by an
// unkillable child or a straggling grandchild.
const killGrace = 5 * time.Second

// Command like exec.Cmd, but with support:
// - io.Closer interface
// - Wait from multiple places
// - Done channel
// - kill escalation: Close() sends the Cancel signal, then after killGrace SIGKILLs the group
type Command struct {
	*exec.Cmd
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	waitDone chan struct{}
}

func NewCommand(s string) *Command {
	ctx, cancel := context.WithCancel(context.Background())
	args := QuoteSplit(s)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.SysProcAttr = procAttr
	// bound Wait() even if the child ignores the kill signal or a grandchild
	// inherited our pipes; exec's killtimeout param overrides this
	cmd.WaitDelay = killGrace
	return &Command{cmd, ctx, cancel, nil, make(chan struct{})}
}

func (c *Command) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}

	go func() {
		c.err = c.Cmd.Wait()
		c.cancel() // release context resources
		close(c.waitDone)
	}()

	return nil
}

func (c *Command) Wait() error {
	<-c.ctx.Done()
	return c.err
}

func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Command) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Command) Close() error {
	c.cancel()
	go c.ensureKill()
	return nil
}

// ensureKill makes sure Close() cannot leave the child, or its descendants,
// running. The context cancel above fires Cmd.Cancel: Process.Kill by
// default, or the configured exec killsignal, which the child is free to
// trap. A killsignal of 15 plus a wrapper that ignores SIGTERM is enough.
// os/exec's WaitDelay then SIGKILLs the direct child, but only the direct
// child, so grandchildren survive a single-pid kill. Once the child is
// reaped, or killGrace expires without a reap, SIGKILL the whole process
// group to sweep stragglers. If that still does not reap it, for example a
// process stuck in uninterruptible sleep, nothing more can be done from
// userspace, and WaitDelay has already unblocked the reader side.
func (c *Command) ensureKill() {
	select {
	case <-c.waitDone:
		// reaped, but a grandchild may have survived: sweep the group
	case <-time.After(killGrace):
		// not even reaped: kill signal ignored or never sent, force it
		if proc := c.Process; proc != nil {
			_ = proc.Kill()
		}
	}

	if proc := c.Process; proc != nil {
		killProcessGroup(proc.Pid)
	}
}
