package shell

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

// Command like exec.Cmd, but with support:
// - io.Closer interface
// - Wait from multiple places
// - Done channel
type Command struct {
	*exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

func NewCommand(s string) *Command {
	ctx, cancel := context.WithCancel(context.Background())
	args := QuoteSplit(s)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.SysProcAttr = procAttr
	cmd.Cancel = func() error {
		return terminateCommand(cmd)
	}
	return &Command{
		Cmd:    cmd,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

func (c *Command) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}

	go func() {
		err := c.Cmd.Wait()
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
		c.cancel() // release context resources
	}()

	return nil
}

func (c *Command) Wait() error {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Command) Done() <-chan struct{} {
	return c.done
}

func (c *Command) Close() error {
	c.cancel()

	select {
	case <-c.done:
		return c.Wait()
	case <-time.After(5 * time.Second):
	}

	_ = killCommand(c.Cmd)

	select {
	case <-c.done:
	case <-time.After(time.Second):
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err != nil {
			return err
		}
		return errors.New("shell: close timeout")
	}

	return c.Wait()
}
