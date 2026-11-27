package dtls

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

func TestAVRecvFrameDataHonorsDeadline(t *testing.T) {
	conn := newRecvDeadlineTestConn(t)
	if err := conn.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	checkRecvFrameError(t, conn, os.ErrDeadlineExceeded)
}

func TestAVRecvFrameDataExpiredDeadline(t *testing.T) {
	conn := newRecvDeadlineTestConn(t)
	if err := conn.SetDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}

	checkRecvFrameError(t, conn, os.ErrDeadlineExceeded)
}

func TestAVRecvFrameDataClearDeadline(t *testing.T) {
	conn := newRecvDeadlineTestConn(t)
	if err := conn.SetDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if ns := conn.recvDeadline.Load(); ns != 0 {
		t.Fatalf("deadline not cleared: %d", ns)
	}

	conn.cancel()
	checkRecvFrameError(t, conn, io.EOF)
}

func TestAVRecvFrameDataCanceled(t *testing.T) {
	conn := newRecvDeadlineTestConn(t)
	if err := conn.SetDeadline(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	conn.cancel()
	checkRecvFrameError(t, conn, io.EOF)
}

func newRecvDeadlineTestConn(t *testing.T) *DTLSConn {
	t.Helper()

	udp, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return &DTLSConn{
		conn:   udp,
		frames: tutk.NewFrameHandler(false),
		ctx:    ctx,
		cancel: cancel,
	}
}

func checkRecvFrameError(t *testing.T, conn *DTLSConn, want error) {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		_, err := conn.AVRecvFrameData()
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AVRecvFrameData is still blocked")
	}
}
