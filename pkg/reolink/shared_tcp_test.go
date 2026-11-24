package reolink

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

func TestSharedTCPKeepsSiblingWhenProfileReopens(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	var logins atomic.Int32
	var handlers sync.WaitGroup
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				return
			}
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				if err := serveSharedTCPFixture(conn, &logins); err != nil &&
					!errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					t.Errorf("camera fixture: %v", err)
				}
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-accepted
		handlers.Wait()
	})

	registry := testRegistry(nil)
	endpoint := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	open := func(route string, history uint8) *baichuanInput {
		t.Helper()
		return openSharedTCPFixture(t, ctx, registry, "viewer:fixture@127.0.0.1:"+endpoint+route, history)
	}
	expect := func(input *baichuanInput, marker byte) {
		t.Helper()
		packet, err := input.Read(ctx)
		if err != nil || packet.Kind != baichuan.MediaVideoP || len(packet.Data) != 8 || packet.Data[7] != marker {
			t.Fatalf("wrong media for preview, want marker %d, got %v (%v)", marker, packet, err)
		}
	}

	main := open("/main?options=sharedtcp:0", 0)
	sub := open("/sub?options=sharedtcp:true", 0)
	if main.shared == nil || main.client != sub.client || logins.Load() != 1 {
		t.Fatal("two compatible previews did not share one login")
	}
	expect(main, 10)
	expect(sub, 11)
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := open("/main?options=sharedtcp:true", 0)
	if reopened.client == sub.client || logins.Load() != 2 {
		t.Fatal("reopened preview reused its retired key on the sibling's socket")
	}
	expect(reopened, 10)
	expect(sub, 21)
	history := open("/extern?options=sharedtcp:true", 1)
	if history.shared != nil || logins.Load() != 3 {
		t.Fatal("history reused the live socket")
	}
	plain := open("/extern", 0)
	if plain.shared != nil || logins.Load() != 4 {
		t.Fatal("the default shared a socket")
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sub.client.Done():
	default:
		t.Fatal("the final shared preview did not release the socket")
	}

	wideMain := open("/main?options=sharedtcp:0", 0)
	wideSub := open("/sub?options=sharedtcp:0", 0)
	teleMain := open("/main?channel=1&options=sharedtcp:1", 0)
	teleSub := open("/sub?channel=1&options=sharedtcp:1", 0)
	if wideMain.client != wideSub.client || teleMain.client != teleSub.client ||
		wideMain.client == teleMain.client || logins.Load() != 6 {
		t.Fatal("the two groups did not use one independent login each")
	}
	// Both otherwise eligible sources must stay outside the first camera's group.
	otherLogin := openSharedTCPFixture(t, ctx, registry, "other:fixture@127.0.0.1:"+endpoint+"/extern?options=sharedtcp:0", 0)
	otherEndpoint := openSharedTCPFixture(t, ctx, registry, "viewer:fixture@localhost:"+endpoint+"/extern?options=sharedtcp:0", 0)
	if otherLogin.client == wideMain.client || otherEndpoint.client == wideMain.client || logins.Load() != 8 {
		t.Fatal("a connection group crossed an endpoint or a login")
	}
	if err := wideMain.client.Close(); err != nil {
		t.Fatal(err)
	}
	expect(teleMain, 10)
	expect(teleSub, 11)
}

func openSharedTCPFixture(t *testing.T, ctx context.Context, registry *Registry, authorityAndRoute string, history uint8) *baichuanInput {
	t.Helper()
	s, err := parseURL("reolink://" + authorityAndRoute)
	if err != nil {
		t.Fatal(err)
	}
	s.preRecord = history
	profile := registry.acquire(registry.cameraKey(s), s)
	t.Cleanup(profile.cancel)
	source, err := openBaichuan(ctx, profile.camera, s)
	if err != nil {
		t.Fatal(err)
	}
	input := source.(*baichuanInput)
	t.Cleanup(func() { _ = input.Close() })
	return input
}

// The fixture uses the plaintext login variant and emits well-formed late
// media after StopPreview. It also sends a marked packet on the active sibling.
func serveSharedTCPFixture(conn net.Conn, logins *atomic.Int32) error {
	active := make(map[[2]byte][20]byte)
	for {
		var request [20]byte
		if _, err := io.ReadFull(conn, request[:]); err != nil {
			return err
		}
		class := binary.LittleEndian.Uint16(request[18:])
		size := int64(binary.LittleEndian.Uint32(request[8:]))
		if class == 0x6414 {
			size += 4
		}
		if _, err := io.CopyN(io.Discard, conn, size); err != nil {
			return err
		}
		const login, preview, stop = 1, 3, 4
		command := binary.LittleEndian.Uint32(request[4:])
		key := [2]byte{request[12], request[13]}
		status := uint16(200)
		var payload []byte
		switch command {
		case login:
			if class == 0x6514 {
				status = 0xdd00
				payload = []byte("<body><Encryption><nonce>0123456789ABCDEF</nonce></Encryption></body>")
			} else {
				logins.Add(1)
			}
		case preview:
			active[key] = request
		case stop:
			delete(active, key)
		case 93: // Keepalive.
		default:
			status = 400
		}
		if err := writeSharedTCPFixture(conn, request, command, status, payload); err != nil {
			return err
		}
		if command == preview {
			if err := writeSharedTCPFixture(conn, request, preview, 200, sharedTCPFixtureVideo(10+key[1])); err != nil {
				return err
			}
		}
		if command == stop && len(active) != 0 {
			if err := writeSharedTCPFixture(conn, request, preview, 200, sharedTCPFixtureVideo(99)); err != nil {
				return err
			}
			for key, other := range active {
				if err := writeSharedTCPFixture(conn, other, preview, 200, sharedTCPFixtureVideo(20+key[1])); err != nil {
					return err
				}
			}
		}
	}
}

func writeSharedTCPFixture(conn net.Conn, incoming [20]byte, command uint32, status uint16, payload []byte) error {
	packet := make([]byte, 24+len(payload))
	copy(packet[:20], incoming[:])
	binary.LittleEndian.PutUint32(packet[4:], command)
	binary.LittleEndian.PutUint32(packet[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint16(packet[16:], status)
	binary.LittleEndian.PutUint16(packet[18:], 0x6414)
	copy(packet[24:], payload)
	_, err := conn.Write(packet)
	return err
}

func sharedTCPFixtureVideo(marker byte) []byte {
	frame := make([]byte, 32)
	binary.LittleEndian.PutUint32(frame, 0x63643130)
	copy(frame[4:8], "H264")
	binary.LittleEndian.PutUint32(frame[8:], 8)
	copy(frame[24:], []byte{0, 0, 0, 1, 0x41, 0x9a, 0x20, marker})
	return frame
}
