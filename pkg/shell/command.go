package shell

import (
	"context"
	"os"
	"os/exec"
)

// Command like exec.Cmd, but with support:
// - io.Closer interface
// - Wait from multiple places
// - Done channel
type Command struct {
	*exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	// Closed only after the wait function has stored err. Unlike ctx.Done(),
	// this channel also guarantees that the process has been waited for.
	done chan struct{}
}

func NewCommand(s string) *Command {
	ctx, cancel := context.WithCancel(context.Background())
	args := QuoteSplit(s)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.SysProcAttr = procAttr
	// Use a platform-specific default. The exec source can override it
	// with killtimeout before Start.
	cmd.WaitDelay = defaultWaitDelay
	c := &Command{Cmd: cmd, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	// Keep the default immediate kill, but target the process group on Linux.
	c.SetKillSignal(os.Kill)
	return c
}

// SetKillSignal configures the initial signal sent on context cancellation.
// Read WaitDelay at cancellation time so that a later configuration of
// killtimeout (before Start) is taken into account. Call before Start.
func (c *Command) SetKillSignal(sig os.Signal) {
	c.Cancel = func() error { return signalProcess(c.Process, sig, c.WaitDelay) }
}

func (c *Command) Start() error {
	// Install the platform-specific cancellation wrapper before os/exec
	// starts its context watcher. On Linux it coordinates signaling and reaping.
	wait := c.prepareWait()

	if err := c.Cmd.Start(); err != nil {
		return err
	}

	go func() {
		c.err = wait()
		c.cancel()    // release context resources
		close(c.done) // publish err to all Wait callers
	}()

	return nil
}

// Wait returns the final result, not just a notification of cancellation.
// Closing done synchronizes the write to err with all readers.
func (c *Command) Wait() error {
	<-c.done
	return c.err
}

func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Done retains the original cancellation semantics: it may close before
// the process exits. Use Wait when the final result is needed.
func (c *Command) Done() <-chan struct{} {
	return c.ctx.Done()
}

// Close requests cancellation without blocking for the grace period.
// Repeated calls are safe because context cancellation is idempotent.
func (c *Command) Close() error {
	c.cancel()
	return nil
}
