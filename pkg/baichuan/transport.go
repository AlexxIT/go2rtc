package baichuan

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

type transport interface {
	io.Reader
	io.Writer
	SetWriteDeadline(time.Time) error
	Close() error
}

func dialTCP(ctx context.Context, cfg Config) (transport, error) {
	dialer := net.Dialer{Timeout: cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.address())
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func writeFull(ctx context.Context, conn transport, timeout time.Duration, b []byte) error {
	deadline := time.Now().Add(timeout)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	var interrupt sync.WaitGroup
	interrupt.Add(1)
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetWriteDeadline(time.Now())
		interrupt.Done()
	})
	defer func() {
		if stop() {
			interrupt.Done()
		}
		interrupt.Wait()
		_ = conn.SetWriteDeadline(time.Time{})
	}()

	for len(b) > 0 {
		n, err := conn.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func interruptDeadline(ctx context.Context, set func(time.Time) error) func() {
	var done sync.WaitGroup
	done.Add(1)
	stop := context.AfterFunc(ctx, func() {
		_ = set(time.Now())
		done.Done()
	})
	return func() {
		if stop() {
			done.Done()
		}
		done.Wait()
		_ = set(time.Time{})
	}
}
