package dtls

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

func TestAVRecvFrameDataHonorsDeadline(t *testing.T) {
	udp, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udp.Close() }()

	// A connected camera that never sends a frame.
	conn := &DTLSConn{conn: udp, frames: tutk.NewFrameHandler(false), ctx: context.Background()}
	if err = conn.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := conn.AVRecvFrameData()
		done <- err
	}()

	select {
	case err = <-done:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("got error %v, want os.ErrDeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AVRecvFrameData is still blocked after the deadline")
	}
}
