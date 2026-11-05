package streams

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestPreloadRequestsDuringDial(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			scheme := "audit-" + strings.ToLower(method)
			slowName := "synthetic-slow-" + method
			otherName := "synthetic-other-" + method
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }

			HandleFunc(scheme, func(source string) (core.Producer, error) {
				if strings.HasSuffix(source, ":slow") {
					close(started)
					<-release
				}
				return nil, errors.New("synthetic dial failure")
			})
			for name, source := range map[string]string{slowName: scheme + ":slow", otherName: scheme + ":fast"} {
				if _, err := New(name, source); err != nil {
					t.Fatal(err)
				}
			}
			defer delete(handlers, scheme)
			defer Delete(slowName)
			defer Delete(otherName)

			server := httptest.NewServer(http.HandlerFunc(apiPreload))
			defer server.Close()
			defer unblock()

			done := make(chan error, 1)
			go func() {
				req, err := http.NewRequest(http.MethodPut, server.URL+"/api/preload?src="+url.QueryEscape(slowName), nil)
				if err != nil {
					done <- err
					return
				}
				resp, err := server.Client().Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode != http.StatusInternalServerError {
						err = errors.New("slow synthetic dial did not report its failure")
					}
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("preload did not reach the synthetic producer")
			}

			client := &http.Client{Timeout: 2 * time.Second}
			defer client.CloseIdleConnections()
			req, err := http.NewRequest(method, server.URL+"/api/preload?src="+url.QueryEscape(otherName), nil)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			resp, requestErr := client.Do(req)
			elapsed := time.Since(start)
			unblock()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if requestErr != nil {
				t.Fatalf("%s for other preloads blocked behind producer dial (%s): %v", method, elapsed, requestErr)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			wantStatus := http.StatusInternalServerError // unrelated synthetic dial / missing preload
			if method == http.MethodGet {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("%s status = %d; want %d", method, resp.StatusCode, wantStatus)
			}
			t.Logf("%s completed while another producer was blocked: %s", method, elapsed)
		})
	}
}

type preloadTestProducer struct {
	core.Connection
	stopped chan struct{}
	once    sync.Once
	onStop  func()
}

func newPreloadTestProducer() *preloadTestProducer {
	return &preloadTestProducer{
		Connection: core.Connection{Medias: []*core.Media{{
			Kind: core.KindVideo, Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{{Name: core.CodecH264, ClockRate: 90000}},
		}}},
		stopped: make(chan struct{}),
	}
}

func (p *preloadTestProducer) Start() error {
	<-p.stopped
	return nil
}

func (p *preloadTestProducer) Stop() error {
	if p.onStop != nil {
		p.onStop()
	}
	err := p.Connection.Stop()
	p.once.Do(func() { close(p.stopped) })
	return err
}

func newPreloadTestStream(t *testing.T, handler Handler) (string, *Stream) {
	t.Helper()
	name := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	HandleFunc(name, handler)
	stream, err := New(name, name+":source")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = DelPreload(name)
		Delete(name)
		delete(handlers, name)
	})
	return name, stream
}

func TestPreloadChangesForSameNameDuringDial(t *testing.T) {
	for _, replace := range []bool{false, true} {
		label := "delete"
		if replace {
			label = "replace"
		}
		t.Run(label, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			first := newPreloadTestProducer()
			second := newPreloadTestProducer()
			var calls atomic.Int32
			name, stream := newPreloadTestStream(t, func(string) (core.Producer, error) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
					return first, nil
				}
				return second, nil
			})
			addDone := make(chan error, 1)
			go func() { addDone <- AddPreload(name, "video") }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("preload did not reach the producer")
			}
			changeStarted := make(chan struct{})
			changeDone := make(chan error, 1)
			go func() {
				close(changeStarted)
				if replace {
					changeDone <- AddPreload(name, "video=h264")
				} else {
					changeDone <- DelPreload(name)
				}
			}()
			<-changeStarted
			select {
			case err := <-changeDone:
				t.Fatalf("change completed before the pending add: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			unblock()
			if err := <-addDone; err != nil {
				t.Fatal(err)
			}
			if err := <-changeDone; err != nil {
				t.Fatal(err)
			}
			select {
			case <-first.stopped:
			default:
				t.Fatal("previous producer was not stopped")
			}
			preload, exists := GetPreloads()[name]
			if exists != replace {
				t.Fatalf("preload exists = %v; want %v", exists, replace)
			}
			wantConsumers := 0
			if replace {
				wantConsumers = 1
				if preload.Query != "video=h264" {
					t.Fatalf("query = %q", preload.Query)
				}
			}
			stream.mu.Lock()
			gotConsumers := len(stream.consumers)
			stream.mu.Unlock()
			if gotConsumers != wantConsumers {
				t.Fatalf("consumers = %d; want %d", gotConsumers, wantConsumers)
			}
		})
	}
}

func TestPreloadRetryAfterFailedDial(t *testing.T) {
	producer := newPreloadTestProducer()
	var calls atomic.Int32
	name, _ := newPreloadTestStream(t, func(string) (core.Producer, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("synthetic dial failure")
		}
		return producer, nil
	})
	if err := AddPreload(name, "video"); err == nil {
		t.Fatal("expected the first dial to fail")
	}
	if _, exists := GetPreloads()[name]; exists {
		t.Fatal("failed preload was registered")
	}
	if err := AddPreload(name, "video"); err != nil {
		t.Fatal(err)
	}
	if _, exists := GetPreloads()[name]; !exists {
		t.Fatal("successful retry was not registered")
	}
	if err := DelPreload(name); err != nil {
		t.Fatal(err)
	}
	select {
	case <-producer.stopped:
	default:
		t.Fatal("producer was not stopped")
	}
}

func TestGetPreloadsDuringProducerStop(t *testing.T) {
	producer := newPreloadTestProducer()
	name, _ := newPreloadTestStream(t, func(string) (core.Producer, error) {
		return producer, nil
	})
	if err := AddPreload(name, "video"); err != nil {
		t.Fatal(err)
	}
	stopping := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	producer.onStop = func() {
		close(stopping)
		<-release
	}
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- DelPreload(name) }()
	select {
	case <-stopping:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not reach the producer")
	}
	listDone := make(chan struct{})
	go func() {
		GetPreloads()
		close(listDone)
	}()
	select {
	case <-listDone:
	case <-time.After(2 * time.Second):
		t.Error("listing preloads blocked behind producer shutdown")
	}
	unblock()
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	<-listDone
	if _, exists := GetPreloads()[name]; exists {
		t.Fatal("deleted preload still registered")
	}
}

func TestPreloadOperationLocksAreReleased(t *testing.T) {
	for i := range 100 {
		name := fmt.Sprintf("missing-%d", i)
		if err := AddPreload(name, "video"); err == nil {
			t.Fatal("unknown stream should fail")
		}
		if err := DelPreload(name); err == nil {
			t.Fatal("unknown preload should fail")
		}
	}
	preloadsMu.Lock()
	remaining := len(preloadLocks)
	preloadsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("completed operations retained %d locks", remaining)
	}
}

func TestPreloadStreamAliases(t *testing.T) {
	var producersMu sync.Mutex
	var producers []*preloadTestProducer
	name, stream := newPreloadTestStream(t, func(string) (core.Producer, error) {
		producer := newPreloadTestProducer()
		producersMu.Lock()
		producers = append(producers, producer)
		producersMu.Unlock()
		return producer, nil
	})
	alias := name + "-alias"
	aliasedStream, err := Patch(alias, name)
	if err != nil {
		t.Fatal(err)
	}
	if aliasedStream != stream {
		t.Fatal("alias does not share the stream")
	}
	t.Cleanup(func() {
		_ = DelPreload(alias)
		Delete(alias)
	})

	runTogether := func(first, second func() error) {
		t.Helper()
		start := make(chan struct{})
		errs := make(chan error, 2)
		for _, operation := range []func() error{first, second} {
			go func() {
				<-start
				errs <- operation()
			}()
		}
		close(start)
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
	}
	assertConsumers := func(want int) {
		t.Helper()
		stream.mu.Lock()
		got := len(stream.consumers)
		stream.mu.Unlock()
		if got != want {
			t.Fatalf("consumers = %d; want %d", got, want)
		}
	}
	runTogether(
		func() error { return AddPreload(name, "video") },
		func() error { return AddPreload(alias, "video=h264") },
	)
	assertConsumers(2)
	if err := DelPreload(alias); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		runTogether(
			func() error { return AddPreload(alias, "video=h264") },
			func() error { return DelPreload(name) },
		)
		assertConsumers(1)
		runTogether(
			func() error { return AddPreload(name, "video") },
			func() error { return DelPreload(alias) },
		)
		assertConsumers(1)
	}
	if err := DelPreload(name); err != nil {
		t.Fatal(err)
	}
	assertConsumers(0)
	for _, key := range []string{name, alias} {
		if _, exists := GetPreloads()[key]; exists {
			t.Fatalf("deleted preload %q still registered", key)
		}
	}
	producersMu.Lock()
	defer producersMu.Unlock()
	for i, producer := range producers {
		select {
		case <-producer.stopped:
		default:
			t.Fatalf("producer %d was not stopped", i)
		}
	}
}

func TestPreloadAliasesSwitchStreams(t *testing.T) {
	firstName, first := newPreloadTestStream(t, func(string) (core.Producer, error) {
		return newPreloadTestProducer(), nil
	})
	secondName := firstName + "-other"
	second, err := New(secondName, firstName+":other")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Delete(secondName) })
	aliases := []string{firstName + "-a", firstName + "-b"}
	for i, source := range []string{firstName, secondName} {
		alias := aliases[i]
		if _, err := Patch(alias, source); err != nil {
			t.Fatal(err)
		}
		if err := AddPreload(alias, "video"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = DelPreload(alias)
			Delete(alias)
		})
	}
	for i, source := range []string{secondName, firstName} {
		if _, err := Patch(aliases[i], source); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, alias := range aliases {
		go func() {
			<-start
			done <- AddPreload(alias, "video=h264")
		}()
	}
	close(start)
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("replacing aliases deadlocked while switching streams")
		}
	}
	for i, stream := range []*Stream{second, first} {
		if preload := GetPreloads()[aliases[i]]; preload == nil || preload.stream != stream {
			t.Fatal("preload did not follow the changed alias")
		}
		stream.mu.Lock()
		consumers := len(stream.consumers)
		stream.mu.Unlock()
		if consumers != 1 {
			t.Fatalf("stream has %d consumers after replacement; want 1", consumers)
		}
		if err := DelPreload(aliases[i]); err != nil {
			t.Fatal(err)
		}
	}
	preloadsMu.Lock()
	remaining := len(preloadLocks)
	preloadsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("completed alias replacements retained %d locks", remaining)
	}
}
