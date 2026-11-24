package baichuan

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestClientPreviewStartsWithFragmentedMedia(t *testing.T) {
	for _, route := range []previewRoute{
		{channel: 0, stream: StreamMain},
		{channel: 1, stream: StreamSub},
		{channel: 1, stream: StreamExtern},
	} {
		for _, lateACK := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%s/lateACK=%t", route.channel, route.stream, lateACK), func(t *testing.T) {
				testClientPreviewStartsWithFragmentedMedia(t, route, lateACK, false)
			})
		}
	}
}

func TestClientPreviewStartsWithUnmarkedPlaintextMedia(t *testing.T) {
	testClientPreviewStartsWithFragmentedMedia(t, previewRoute{stream: StreamMain}, false, true)
}

func testClientPreviewStartsWithFragmentedMedia(t *testing.T, route previewRoute, lateACK, unmarked bool) {
	clientConn, camera := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, clientConn)
	client.logged = unmarked
	cameraErr := make(chan error, 1)
	t.Cleanup(func() {
		_ = client.Close()
		_ = camera.Close()
		if err := <-cameraErr; err != nil {
			t.Error(err)
		}
	})
	_ = camera.SetDeadline(time.Now().Add(3 * time.Second))
	// One protocol-valid I body, split into a prefix, remainder and full chunk.
	// Codec decoding is outside this client/parser routing regression.
	video := bytes.Repeat([]byte{0x55}, 1024+13714+39850)
	binary.LittleEndian.PutUint32(video, mediaIFrameMin)
	copy(video[4:8], "H265")
	binary.LittleEndian.PutUint32(video[8:12], uint32(len(video)-28))
	binary.LittleEndian.PutUint32(video[12:16], 4)
	binary.LittleEndian.PutUint32(video[16:20], 123456)
	binary.LittleEndian.PutUint32(video[24:28], 1000)
	copy(video[28:], []byte{0, 0, 0, 1, 0x26, 1, 0x80})
	go func() {
		var state cipherState
		var err error
		if !unmarked {
			state, err = serveLogin(camera, cfg)
		}
		if err == nil {
			err = serveFragmentedPreview(camera, cfg, state, video, lateACK, unmarked)
		}
		cameraErr <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	preview, err := client.StartPreview(ctx, route.channel, route.stream)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []MediaKind{MediaInfo, MediaVideoI, MediaInfo} {
		packet, err := preview.Read(ctx)
		if err != nil || packet.Kind != kind {
			t.Fatalf("packet kind=%d, want %d: %v", packet.Kind, kind, err)
		}
		if kind == MediaVideoI && (packet.ShortVideo || packet.Timestamp != 123456 || !bytes.Equal(packet.Data, video[28:])) {
			t.Fatalf("startup I changed: short=%t bytes=%d, want %d", packet.ShortVideo, len(packet.Data), len(video)-28)
		}
	}
}

func serveFragmentedPreview(conn net.Conn, cfg Config, state cipherState, video []byte, lateACK, unmarked bool) error {
	request, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return err
	}
	msg, err := decodeFrame(request, state, false)
	if err != nil {
		return err
	}
	if msg.header.Command != commandPreview {
		return fmt.Errorf("expected preview request, got %d", msg.header.Command)
	}
	h := header{Command: commandPreview, Channel: msg.header.Channel, Stream: msg.header.Stream,
		Sequence: msg.header.Sequence, ResponseCode: 200}
	for i, part := range []struct {
		data    []byte
		flagged bool
	}{
		{infoFixture(), true}, {video[:1024], true},
		{video[1024:14738], false}, {video[14738:], false}, {infoFixture(), true},
	} {
		ext := []byte("<Extension><checkPos>0</checkPos></Extension>")
		data := part.data
		if unmarked && i == 0 {
			if err := writeTestFrame(conn, h, nil, data[:8], state, true); err != nil {
				return err
			}
			data = data[8:]
		}
		if part.flagged && !unmarked {
			ext = []byte(fmt.Sprintf("<Extension><binaryData>1</binaryData><encryptLen>%d</encryptLen></Extension>", len(data)))
			data = testCrypt(state, h.Channel, data, true)
		}
		if err := writeTestFrame(conn, h, ext, data, state, true); err != nil {
			return err
		}
		if lateACK && i == 1 {
			ext := []byte("<Extension><binaryData>0</binaryData></Extension>")
			if err := writeTestFrame(conn, h, ext, []byte("<body/>"), state, false); err != nil {
				return err
			}
		}
	}
	// Keep the transport alive until cleanup closes the client and joins its reader.
	_, _ = readFrame(conn, cfg.Limits)
	return nil
}
