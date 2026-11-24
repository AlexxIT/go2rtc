package reolink

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestRegistryFailedOpenWaitsForRetryDelay(t *testing.T) {
	cause := errors.New("open failed")
	registry := testRegistry(func(context.Context, *Camera, source) (profileInput, error) {
		return nil, cause
	})
	started := time.Now()
	if _, err := registry.Dial("reolink://u:p@camera/main?backchannel=0"); !errors.Is(err, cause) {
		t.Fatalf("Dial lost open error: %v", err)
	}
	if elapsed := time.Since(started); elapsed < profileRetryDelay {
		t.Fatalf("failed open returned after %s, want at least %s", elapsed, profileRetryDelay)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.cameras) != 0 {
		t.Fatal("failed open retained its camera")
	}
}

func TestProducerStartWaitsForImmediateTerminalFailure(t *testing.T) {
	opener := &fakeProfileOpener{}
	producer, err := testRegistry(opener.open).Dial("reolink://u:p@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Stop()

	input := opener.input(0)
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	select {
	case <-producer.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not terminate promptly")
	}
	if elapsed := time.Since(producer.attemptBegan); elapsed >= profileRetryDelay {
		t.Fatalf("fixture terminated too late to exercise guard: %s", elapsed)
	}
	if err = producer.Start(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Start lost terminal error: %v", err)
	}
	if elapsed := time.Since(producer.attemptBegan); elapsed < profileRetryDelay {
		t.Fatalf("Start returned after %s, want at least %s", elapsed, profileRetryDelay)
	}
	if !input.aborted.Load() {
		t.Fatal("Start returned before failed input teardown")
	}
	registry := producer.camera.registry
	registry.mu.Lock()
	activeSessions := producer.camera.mainSessions + producer.camera.otherSessions
	registry.mu.Unlock()
	if activeSessions != 0 {
		t.Fatal("Start returned with an active camera session")
	}
	if err = producer.Stop(); err != nil {
		t.Fatalf("Stop after terminal failure: %v", err)
	}
	if err = producer.Stop(); err != nil {
		t.Fatalf("repeated Stop after terminal failure: %v", err)
	}
}

type observedFailureDoneContext struct {
	context.Context
	requested chan struct{}
	resume    chan struct{}
	once      sync.Once
}

func (c *observedFailureDoneContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.requested)
		<-c.resume
	})
	return c.Context.Done()
}

func TestProducerStopCancelsRetryWait(t *testing.T) {
	opener := &fakeProfileOpener{}
	producer, err := testRegistry(opener.open).Dial("reolink://u:p@camera/main?backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Stop()
	input := opener.input(0)
	input.results <- inputResult{err: io.ErrUnexpectedEOF}
	select {
	case <-producer.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not terminate promptly")
	}
	if time.Since(producer.attemptBegan) >= profileRetryDelay {
		t.Fatal("fixture terminated too late to exercise cancellation")
	}

	stopCtx := &observedFailureDoneContext{
		Context: producer.stopCtx, requested: make(chan struct{}), resume: make(chan struct{}),
	}
	producer.stopCtx = stopCtx
	waitDone := make(chan struct{})
	go func() {
		producer.waitRetry()
		close(waitDone)
	}()
	select {
	case <-stopCtx.requested:
	case <-waitDone:
		t.Fatal("retry wait completed before observing Stop intent")
	case <-time.After(5 * time.Second):
		t.Fatal("retry wait did not inspect Stop intent")
	}
	err = producer.Stop()
	close(stopCtx.resume)
	remaining := time.Until(producer.attemptBegan.Add(profileRetryDelay))
	if err != nil {
		t.Fatal(err)
	}
	if remaining <= profileRetryDelay/2 {
		<-waitDone
		t.Fatal("fixture left too little time to distinguish cancellation from timer expiry")
	}
	select {
	case <-waitDone:
	case <-time.After(profileRetryDelay / 2):
		t.Fatal("Stop did not cancel the retry wait")
	}
}
