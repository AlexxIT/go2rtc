package shell

import (
	"context"
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
}

func NewCommand(s string) *Command {
	ctx, cancel := context.WithCancel(context.Background())
	args := QuoteSplit(s)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.SysProcAttr = procAttr
	return &Command{cmd, ctx, cancel, nil}
}

func (c *Command) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}

	go func() {
		c.err = c.Cmd.Wait()
		c.cancel() // release context resources
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
	return nil
}
