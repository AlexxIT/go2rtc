package reolink

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type shutdownInput struct {
	*fakeProfileInput
	abortErr error
}

func (i *shutdownInput) Close() error {
	<-i.done
	return i.abortErr
}

func (i *shutdownInput) Abort() error {
	_ = i.fakeProfileInput.Abort()
	return i.abortErr
}

func TestProducerStopAbortsAndCachesError(t *testing.T) {
	abortErr := errors.New("transport close failed")
	input := &shutdownInput{fakeProfileInput: newFakeProfileInput(), abortErr: abortErr}
	registry := testRegistry(func(context.Context, *Camera, source) (profileInput, error) { return input, nil })
	var logs bytes.Buffer
	registry.log = zerolog.New(&logs).Level(zerolog.WarnLevel)
	producer, err := registry.Dial("reolink://test:test@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	// Hold the first talk lock until the owned transport is aborted.
	producer.talkMu.Lock()
	go func() {
		<-input.done
		producer.talkMu.Unlock()
	}()
	results := make(chan error, 4)
	for range cap(results) {
		go func() { results <- producer.Stop() }()
	}
	var first error
	for range cap(results) {
		select {
		case err = <-results:
			if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, abortErr) {
				t.Fatalf("missing shutdown error: %v", err)
			}
			if first == nil {
				first = err
			} else if err != first {
				t.Fatal("concurrent Stop returned a different cached error")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Stop did not abort the owned input before waiting for talk locks")
		}
	}
	if producer.Stop() != first {
		t.Fatal("repeated Stop returned a different cached error")
	}
	text := logs.String()
	if strings.Count(text, `"message":"reolink shutdown grace expired"`) != 1 ||
		!strings.Contains(text, `"level":"warn"`) || !strings.Contains(text, `"error_class":"timeout"`) ||
		!strings.Contains(text, `"camera":"camera:9000"`) || !strings.Contains(text, `"channel":0`) ||
		!strings.Contains(text, `"profile":"mainStream"`) || strings.Contains(text, "reolink://") {
		t.Fatalf("unexpected shutdown warning: %s", text)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.cameras) != 0 {
		t.Fatal("Stop retained its camera session")
	}
}
