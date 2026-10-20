package baichuan

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func keepAliveTestClient(t *testing.T, status uint16) (*Client, *Preview) {
	t.Helper()
	conn, peer := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 20 * time.Millisecond
	client := newClient(context.Background(), cfg, conn)
	t.Cleanup(func() {
		_ = client.Close()
		_ = peer.Close()
	})
	go func() {
		for {
			frame, err := readFrame(peer, cfg.Limits)
			if err != nil {
				return
			}
			if status != 0 {
				if err = writeTestFrame(peer, header{
					Command: commandPing, Sequence: frame.header.Sequence,
					ResponseCode: status, Class: classOffset,
				}, nil, nil, cipherState{}, false); err != nil {
					return
				}
			}
		}
	}()
	parser, err := NewMediaParser(cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	preview := &Preview{
		client: client, key: previewKey{channel: 0, stream: 0},
		messages:   make(chan message, previewQueueMessages),
		queueLimit: int64(cfg.Limits.MaxMediaBuffer), done: make(chan struct{}), parser: parser,
	}
	if err := client.addPreview(preview); err != nil {
		t.Fatal(err)
	}
	return client, preview
}

func keepAliveVideoFrame() []byte {
	frame := make([]byte, 32)
	binary.LittleEndian.PutUint32(frame, mediaPFrameMin)
	copy(frame[4:8], "H264")
	binary.LittleEndian.PutUint32(frame[8:12], 8)
	copy(frame[24:], []byte{0, 0, 0, 1, 0x41, 0x9a, 0x20, 0})
	return frame
}

func TestKeepAliveDoesNotKillAdvancingPreview(t *testing.T) {
	client, preview := keepAliveTestClient(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	failures := 0
	lastPackets := client.mediaPackets.Load()
	for range 3 {
		preview.deliver(message{payload: keepAliveVideoFrame()})
		packet, err := preview.Read(ctx)
		if err != nil || packet.Kind != MediaVideoP || len(packet.Data) != 8 {
			t.Fatalf("valid preview packet: kind=%d size=%d err=%v", packet.Kind, len(packet.Data), err)
		}
		if client.keepAliveStep(&failures, &lastPackets) {
			t.Fatal("unanswered pings killed an advancing preview")
		}
	}
	select {
	case <-client.Done():
		t.Fatal("advancing preview was closed")
	default:
	}
}

func TestKeepAliveClosesWithoutCompleteMediaProgress(t *testing.T) {
	for _, name := range []string{"no-preview", "info-only", "partial-video", "stalled-video"} {
		t.Run(name, func(t *testing.T) {
			client, preview := keepAliveTestClient(t, 0)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			switch name {
			case "no-preview":
				preview.closeLocal(context.Canceled)
			case "info-only":
				preview.deliver(message{payload: infoFixture()})
				if packet, err := preview.Read(ctx); err != nil || packet.Kind != MediaInfo {
					t.Fatalf("info packet: kind=%d err=%v", packet.Kind, err)
				}
			case "partial-video":
				readDone := make(chan error, 1)
				go func() { _, err := preview.Read(ctx); readDone <- err }()
				defer func() {
					cancel()
					<-readDone
				}()
			case "stalled-video":
				preview.deliver(message{payload: keepAliveVideoFrame()})
				if packet, err := preview.Read(ctx); err != nil || packet.Kind != MediaVideoP {
					t.Fatalf("video packet: kind=%d err=%v", packet.Kind, err)
				}
			}

			failures := 0
			lastPackets := client.mediaPackets.Load()
			for i := range 3 {
				if name == "partial-video" {
					preview.deliver(message{payload: keepAliveVideoFrame()[:4]})
				}
				stopped := client.keepAliveStep(&failures, &lastPackets)
				if stopped != (i == 2) {
					t.Fatalf("ping failure %d: stopped=%t", i+1, stopped)
				}
			}
			if err := client.Err(); err == nil || !strings.Contains(err.Error(), "keepalive failed") ||
				!errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("unexpected terminal error: %v", err)
			}
		})
	}
}

func TestKeepAliveDoesNotExcuseCameraRejection(t *testing.T) {
	client, preview := keepAliveTestClient(t, 403)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	failures := 0
	lastPackets := client.mediaPackets.Load()
	for i := range 3 {
		preview.deliver(message{payload: keepAliveVideoFrame()})
		if packet, err := preview.Read(ctx); err != nil || packet.Kind != MediaVideoP {
			t.Fatalf("video packet: kind=%d err=%v", packet.Kind, err)
		}
		stopped := client.keepAliveStep(&failures, &lastPackets)
		if stopped != (i == 2) {
			t.Fatalf("rejected ping %d: stopped=%t", i+1, stopped)
		}
	}
	var status *StatusError
	if !errors.As(client.Err(), &status) || status.Code != 403 {
		t.Fatalf("unexpected terminal error: %v", client.Err())
	}
}

func TestKeepAliveWriteFailureClosesWithMediaProgress(t *testing.T) {
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, &failingTransport{closed: make(chan struct{})})
	client.mediaPackets.Add(1)
	failures, lastPackets := 0, uint64(0)
	_ = client.keepAliveStep(&failures, &lastPackets)
	if !errors.Is(client.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("write failure was excused by media progress: %v", client.Err())
	}
	_ = client.Close()
}

func TestCanceledRequestsPreserveClient(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pending bool
		send    func(*Client, context.Context) error
	}{
		{
			name:    "round-trip",
			pending: true,
			send: func(client *Client, ctx context.Context) error {
				_, err := client.roundTrip(ctx, request{command: commandPing, class: classOffset})
				return err
			},
		},
		{
			name: "write-request",
			send: func(client *Client, ctx context.Context) error {
				return client.writeRequest(ctx, request{command: commandTalkData, class: classOffset})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, cameraConn := net.Pipe()
			cfg, err := NewConfig("camera.local", "admin", "password").normalized()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Timeout = time.Second
			client := newClient(context.Background(), cfg, clientConn)
			defer client.Close()
			defer cameraConn.Close()

			ctx, cancel := context.WithCancel(context.Background())
			client.sendMu.Lock()
			locked := true
			defer func() {
				cancel()
				if locked {
					client.sendMu.Unlock()
				}
			}()
			client.reqMu.Lock()
			sequence := client.seq
			client.reqMu.Unlock()
			requestErr := make(chan error, 1)
			go func() { requestErr <- tc.send(client, ctx) }()
			waitFor(t, time.Second, func() bool {
				client.reqMu.Lock()
				defer client.reqMu.Unlock()
				if tc.pending {
					return len(client.reqs) != 0
				}
				return client.seq != sequence
			})
			cancel()
			client.sendMu.Unlock()
			locked = false
			select {
			case err = <-requestErr:
			case <-time.After(time.Second):
				t.Fatal("canceled request remained blocked")
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("unexpected canceled request error: %v", err)
			}
			select {
			case <-client.Done():
				t.Fatalf("client shut down after canceled request: %v", client.Err())
			default:
			}
			client.reqMu.Lock()
			pending := len(client.reqs)
			client.reqMu.Unlock()
			if pending != 0 {
				t.Fatalf("canceled request retained %d reservations", pending)
			}

			cameraErr := make(chan error, 1)
			go func() {
				value, err := readFrame(cameraConn, cfg.Limits)
				if err == nil && (value.header.Command != commandPing || value.header.Sequence != sequence+1) {
					err = errors.New("unexpected request before follow-up ping")
				}
				if err == nil {
					err = writeTestFrame(cameraConn, header{
						Command: commandPing, Sequence: value.header.Sequence, ResponseCode: 200,
					}, nil, nil, cipherState{}, false)
				}
				cameraErr <- err
			}()
			if _, err := client.roundTrip(context.Background(), request{command: commandPing, class: classOffset}); err != nil {
				t.Fatalf("client unusable after cancellation: %v", err)
			}
			if err := <-cameraErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}
