package dtls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pion "github.com/pion/dtls/v3"
)

func TestIOCtrlBufferedResponseCannotSuppressInitialRequest(t *testing.T) {
	responses := make(chan []byte, 1)
	responses <- []byte("matching response")
	var writes [][]byte
	write := func(data []byte) error {
		writes = append(writes, append([]byte(nil), data...))
		return nil
	}
	match := func(data []byte) bool {
		if len(writes) == 0 || !bytes.Equal(writes[0], []byte("request")) {
			t.Fatal("buffered response accepted before the outbound request")
		}
		return string(data) == "matching response"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := writeAndWaitIOCtrl(ctx, responses, []byte("request"), match, write, func() []byte { return []byte("ack") }, 10*time.Millisecond)
	if err != nil || string(got) != "matching response" || len(writes) != 2 || string(writes[1]) != "ack" {
		t.Fatal("initial request / response acknowledgement ordering changed")
	}
}

func TestIOCtrlAcknowledgesUnmatchedResponsesAndKeepsWaiting(t *testing.T) {
	responses := make(chan []byte, 2)
	responses <- []byte("unrelated")
	responses <- []byte("expected")
	var writes [][]byte
	write := func(data []byte) error {
		writes = append(writes, append([]byte(nil), data...))
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := writeAndWaitIOCtrl(ctx, responses, []byte("request"), func(data []byte) bool { return string(data) == "expected" }, write, func() []byte { return []byte("ack") }, time.Second)
	if err != nil || string(got) != "expected" || len(writes) != 3 || string(writes[1]) != "ack" || string(writes[2]) != "ack" {
		t.Fatal("unmatched response consumed the transaction or was not acknowledged")
	}
}

func TestIOCtrlWriteFailureDoesNotConsumeBufferedResponseOrLeakText(t *testing.T) {
	responses := make(chan []byte, 1)
	responses <- []byte("expected")
	writes := 0
	write := func([]byte) error { writes++; return errors.New("PRIVATE_WRITER_ERROR") }
	_, err := writeAndWaitIOCtrl(context.Background(), responses, []byte("request"), func([]byte) bool { return true }, write, func() []byte { return []byte("ack") }, time.Second)
	if err == nil || err.Error() != "control request write failed" || strings.Contains(err.Error(), "PRIVATE") || writes != 1 || len(responses) != 1 {
		t.Fatal("initial write failure was hidden, consumed a response or leaked upstream text")
	}
}

func TestIOCtrlAcknowledgementFailureIsFixed(t *testing.T) {
	responses := make(chan []byte, 1)
	responses <- []byte("expected")
	writes := 0
	write := func([]byte) error {
		writes++
		if writes == 2 {
			return errors.New("PRIVATE_ACK_ERROR")
		}
		return nil
	}
	_, err := writeAndWaitIOCtrl(context.Background(), responses, []byte("request"), func([]byte) bool { return true }, write, func() []byte { return []byte("ack") }, time.Second)
	if err == nil || err.Error() != "control acknowledgement write failed" {
		t.Fatal("acknowledgement write failure was hidden or leaked upstream text")
	}
}

func TestIOCtrlOwnedRetryStopsAllWritesBeforeReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var writes atomic.Int32
	write := func([]byte) error {
		if writes.Add(1) == 2 {
			cancel()
		}
		return nil
	}
	_, err := writeAndWaitIOCtrl(ctx, make(chan []byte), []byte("request"), func([]byte) bool { return true }, write, func() []byte { return []byte("ack") }, 5*time.Millisecond)
	if !errors.Is(err, context.Canceled) || writes.Load() != 2 {
		t.Fatal("initial write / retry / cancellation behavior changed")
	}
	time.Sleep(30 * time.Millisecond)
	if writes.Load() != 2 {
		t.Fatal("a retry wrote after the transaction returned")
	}
}

func TestIOCtrlTimeoutAndClosedResponseChannel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	writes := 0
	write := func([]byte) error { writes++; return nil }
	_, err := writeAndWaitIOCtrl(ctx, make(chan []byte), nil, func([]byte) bool { return true }, write, func() []byte { return nil }, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) || writes != 1 {
		t.Fatal("timeout did not stop the wait after its synchronous initial write")
	}
	closed := make(chan []byte)
	close(closed)
	_, err = writeAndWaitIOCtrl(context.Background(), closed, nil, func([]byte) bool { return true }, write, func() []byte { return nil }, time.Hour)
	if !errors.Is(err, io.EOF) {
		t.Fatal("closed control receive channel did not terminate")
	}
}

func TestIOCtrlCancellationBeforeAndDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write := func([]byte) error { t.Fatal("cancelled transaction wrote a request"); return nil }
	if _, err := writeAndWaitIOCtrl(ctx, nil, nil, nil, write, nil, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal("pre-cancelled context did not stop initial write")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	written := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		_, err := writeAndWaitIOCtrl(ctx, make(chan []byte), nil, func([]byte) bool { return true }, func([]byte) error { written <- struct{}{}; return nil }, func() []byte { return nil }, time.Hour)
		result <- err
	}()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("initial write never happened")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation was not preserved")
		}
	case <-time.After(time.Second):
		t.Fatal("context cancellation did not wake the response wait")
	}
}

func TestIOCtrlUnavailableConnectionAndConcurrentTransactionFailImmediately(t *testing.T) {
	c := &DTLSConn{ctx: context.Background(), rawCmd: make(chan []byte, 1)}
	c.rawCmd <- []byte("stale response")
	if _, err := c.WriteAndWaitIOCtrl(nil, func([]byte) bool { return true }, time.Second); err == nil || err.Error() != "control connection unavailable" || len(c.rawCmd) != 1 || c.avSeq != 0 {
		t.Fatal("unavailable connection accepted a stale response or allocated a frame")
	}
	// The busy guard must return before using this uninitialized DTLS connection.
	c.clientConn = &pion.Conn{}
	c.ioctlMu.Lock()
	_, err := c.WriteAndWaitIOCtrl(nil, func([]byte) bool { return true }, time.Second)
	c.ioctlMu.Unlock()
	if err == nil || err.Error() != "control transaction busy" || c.avSeq != 0 {
		t.Fatal("concurrent control transaction entered the shared response queue")
	}
}
