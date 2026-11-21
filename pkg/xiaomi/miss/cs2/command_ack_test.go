package cs2

import (
	"net"
	"testing"
	"time"
)

func TestCommandResponseAndAckLifecycle(t *testing.T) {
	for _, command := range []uint32{0x100, 0x1001} {
		left, right := net.Pipe()
		c := &Conn{Conn: left}
		go func() {
			buf := make([]byte, 256)
			_, _ = right.Read(buf)
			c.signalCmdAck(true)
			if command != 0x100 {
				time.Sleep(30 * time.Millisecond)
				c.signalCmdAck(false)
			}
		}()
		start := time.Now()
		if err := c.WriteCommand(command, []byte("request")); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		if elapsed > 500*time.Millisecond {
			t.Fatalf("ACK was delayed: %v", elapsed)
		}
		if command != 0x100 && elapsed < 20*time.Millisecond {
			t.Fatal("unrelated auth response acknowledged another command")
		}
		c.signalCmdAck(false) // A late ACK must have no live callback/timer.
		c.ackMu.Lock()
		if c.cmdAck != nil {
			t.Error("ACK callback retained after command")
		}
		c.ackMu.Unlock()
		_ = left.Close()
		_ = right.Close()
	}
}
