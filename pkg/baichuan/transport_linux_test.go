package baichuan

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type quickACKReader struct {
	net.Conn
	n   int
	err error
}

func (c quickACKReader) Read(b []byte) (int, error) {
	copy(b, "abc")
	return c.n, c.err
}

func TestQuickACKRead(t *testing.T) {
	for _, test := range []struct {
		n   int
		err error
	}{{3, nil}, {3, io.EOF}, {0, nil}, {0, io.EOF}} {
		calls := 0
		c := &quickACKConn{Conn: quickACKReader{n: test.n, err: test.err}, force: func() error {
			calls++
			return nil
		}}
		n, err := c.Read(make([]byte, 3))
		if n != test.n || err != test.err || (calls == 1) != (test.n > 0) {
			t.Fatalf("read=%d/%v calls=%d", n, err, calls)
		}
	}
}

func TestQuickACKFailureDisablesConcurrentReads(t *testing.T) {
	calls := 0
	c := &quickACKConn{Conn: quickACKReader{n: 3, err: io.EOF}, force: func() error {
		calls++
		return errors.New("option unavailable")
	}}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := c.Read(make([]byte, 3))
			if n != 3 || err != io.EOF {
				t.Errorf("read=%d/%v", n, err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 || c.force != nil {
		t.Fatalf("option calls=%d", calls)
	}
}

func quickACKPair(t testing.TB) (*net.TCPConn, net.Conn, uint16) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	peer, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	return conn.(*net.TCPConn), peer, uint16(l.Addr().(*net.TCPAddr).Port)
}

func TestQuickACKTCP(t *testing.T) {
	conn, peer, port := quickACKPair(t)
	c := newQuickACKConn(conn).(*quickACKConn)
	if err := c.force(); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 3)
	if n, err := io.ReadFull(c, b); n != 3 || err != nil || string(b) != "abc" {
		t.Fatalf("read=%d/%v", n, err)
	}
	for _, enabled := range []bool{false, true} {
		cfg := NewConfig("127.0.0.1", "", "")
		cfg.Port, cfg.TCPQuickACK = port, enabled
		transport, err := dialTCP(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, wrapped := transport.(*quickACKConn)
		_ = transport.Close()
		if wrapped != enabled {
			t.Fatalf("wrapped=%t enabled=%t", wrapped, enabled)
		}
	}
}

func TestQuickACKCloseAndFallback(t *testing.T) {
	plain, other := net.Pipe()
	defer plain.Close()
	defer other.Close()
	if newQuickACKConn(plain) != plain {
		t.Fatal("wrapped non-TCP transport")
	}
	conn, _, _ := quickACKPair(t)
	c := newQuickACKConn(conn).(*quickACKConn)
	done := make(chan error, 1)
	go func() { _, err := c.Read(make([]byte, 1)); done <- err }()
	_ = c.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not interrupt read")
	}
	// The real RawConn.Control failure must not replace a positive read's error.
	c.Conn = quickACKReader{n: 3, err: io.EOF}
	if n, err := c.Read(make([]byte, 3)); n != 3 || err != io.EOF || c.force != nil {
		t.Fatalf("closed socket fallback=%d/%v", n, err)
	}
}
