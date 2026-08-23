package baichuan

import (
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClientLoginAndPreview(t *testing.T) {
	for _, route := range []previewRoute{
		{channel: 0, stream: StreamMain, wire: 0, handle: 0},
		{channel: 3, stream: StreamSub, wire: 1, handle: 256},
		{channel: 7, stream: StreamExtern, wire: 2, handle: 1024},
	} {
		t.Run(string(route.stream), func(t *testing.T) {
			clientConn, cameraConn := net.Pipe()
			cfg, err := NewConfig("camera.local", "admin", "password").normalized()
			if err != nil {
				t.Fatal(err)
			}
			client := newClient(context.Background(), cfg, clientConn)
			defer client.Close()
			cameraErr := make(chan error, 1)
			go func() { cameraErr <- servePreview(cameraConn, cfg, route) }()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			preview, err := client.StartPreview(ctx, route.channel, route.stream)
			if err != nil {
				t.Fatalf("%v; camera: %v", err, <-cameraErr)
			}
			packet, err := preview.Read(ctx)
			if err != nil || packet.Kind != MediaInfo || packet.Width != 3840 || packet.Height != 2160 {
				t.Fatalf("unexpected info packet: %+v, %v", packet, err)
			}
			packet, err = preview.Read(ctx)
			if err != nil || packet.Kind != MediaVideoI || packet.Codec != "H265" {
				t.Fatalf("unexpected video packet: %+v, %v", packet, err)
			}
			if err = preview.Close(); err != nil {
				t.Fatal(err)
			}
			if err = preview.Close(); err != nil {
				t.Fatalf("second close: %v", err)
			}
			if err = <-cameraErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRoundTripUsesDefaultTimeout(t *testing.T) {
	clientConn, cameraConn := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 20 * time.Millisecond
	client := newClient(context.Background(), cfg, clientConn)
	defer client.Close()
	read := make(chan error, 1)
	go func() {
		_, err := readFrame(cameraConn, cfg.Limits)
		read <- err
	}()
	_, err = client.roundTrip(context.Background(), request{command: commandPing, class: classOffset})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected response timeout: %v", err)
	}
	if err = <-read; err != nil {
		t.Fatal(err)
	}
	_ = cameraConn.Close()
}

func TestClientReserveWrapAndLimit(t *testing.T) {
	c := &Client{cfg: Config{Limits: Limits{MaxPending: 2}}, seq: ^uint16(0), reqs: make(map[pendingKey]chan message)}
	first, _, err := c.reserve(commandPing)
	if err != nil || first.sequence != ^uint16(0) {
		t.Fatalf("unexpected first reservation: %+v %v", first, err)
	}
	second, _, err := c.reserve(commandPing)
	if err != nil || second.sequence != 0 {
		t.Fatalf("unexpected wrapped reservation: %+v %v", second, err)
	}
	if _, _, err = c.reserve(commandPing); err == nil {
		t.Fatal("accepted reservation beyond pending limit")
	}
}

type previewRoute struct {
	channel uint8
	stream  Stream
	wire    uint8
	handle  uint32
}

func servePreview(conn net.Conn, cfg Config, route previewRoute) error {
	defer conn.Close()
	aes, err := serveLogin(conn, cfg)
	if err != nil {
		return err
	}
	return servePreviewSession(conn, cfg, aes, route)
}

func servePreviewSession(conn net.Conn, cfg Config, aes cipherState, route previewRoute) error {
	previewRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return err
	}
	preview, err := decodeFrame(previewRequest, aes, false)
	if err != nil {
		return err
	}
	var body previewEnvelope
	if err = xml.Unmarshal(preview.payload, &body); err != nil ||
		preview.header.Command != commandPreview || preview.header.Channel != route.channel ||
		preview.header.Stream != route.wire || body.Preview.Channel != route.channel ||
		body.Preview.Stream != route.stream || body.Preview.Handle != route.handle {
		return fmt.Errorf("invalid preview request: %+v %s", preview.header, preview.payload)
	}
	if err = writeTestFrame(conn, header{
		Command: commandPreview, Channel: route.channel, Stream: route.wire, Sequence: preview.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, nil, nil, aes, false); err != nil {
		return err
	}

	media := append(infoFixture(), videoFixture()...)
	media = append(media, infoFixture()...)
	if err = writeTestFrame(conn, header{
		Command: commandPreview, Channel: route.channel, Stream: route.wire, Sequence: preview.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, []byte("<body><binaryData>1</binaryData></body>"), media, aes, true); err != nil {
		return err
	}

	stopRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return err
	}
	stop, err := decodeFrame(stopRequest, aes, false)
	if err != nil {
		return err
	}
	var stopBody stopPreviewEnvelope
	if err = xml.Unmarshal(stop.payload, &stopBody); err != nil ||
		stop.header.Command != commandStopPreview || stop.header.Channel != route.channel ||
		stop.header.Stream != route.wire || stopBody.Preview.Channel != route.channel ||
		stopBody.Preview.Handle != route.handle {
		return fmt.Errorf("invalid stop request: %+v %s", stop.header, stop.payload)
	}
	return writeTestFrame(conn, header{
		Command: commandStopPreview, Channel: route.channel, Stream: route.wire, Sequence: stop.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, nil, nil, aes, false)
}

func serveLogin(conn net.Conn, cfg Config) (cipherState, error) {

	nonceRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return cipherState{}, err
	}
	if nonceRequest.header.Command != commandLogin || nonceRequest.header.ResponseCode != 0xdc12 ||
		nonceRequest.header.Class != classLegacy || len(nonceRequest.body) != 0 {
		return cipherState{}, fmt.Errorf("invalid nonce request: %+v", nonceRequest.header)
	}
	const nonce = "0123456789ABCDEF"
	bc := cipherState{mode: encryptionBC}
	if err = writeTestFrame(conn, header{
		Command: commandLogin, Sequence: nonceRequest.header.Sequence,
		ResponseCode: 0xdd12, Class: classOffset,
	}, nil, []byte("<body><Encryption><nonce>"+nonce+"</nonce></Encryption></body>"), bc, false); err != nil {
		return cipherState{}, err
	}

	loginRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return cipherState{}, err
	}
	login, err := decodeFrame(loginRequest, bc, false)
	if err != nil {
		return cipherState{}, err
	}
	loginXML := string(login.payload)
	if !strings.Contains(loginXML, "<LoginUser") || strings.Contains(loginXML, ">admin<") || strings.Contains(loginXML, ">password<") {
		return cipherState{}, fmt.Errorf("invalid login XML: %s", loginXML)
	}
	if err = writeTestFrame(conn, header{
		Command: commandLogin, Sequence: loginRequest.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, nil, nil, bc, false); err != nil {
		return cipherState{}, err
	}

	aes := cipherState{mode: encryptionAES, aesKey: deriveAESKey(nonce, cfg.password), hasKey: true}
	return aes, nil
}

func writeTestFrame(conn net.Conn, h header, ext, payload []byte, state cipherState, binaryPayload bool) error {
	ext = testCrypt(state, h.Channel, ext, true)
	if !binaryPayload {
		payload = testCrypt(state, h.Channel, payload, true)
	}
	bodyLen := len(ext) + len(payload)
	packet := make([]byte, 24+bodyLen)
	binary.LittleEndian.PutUint32(packet, wireMagic)
	binary.LittleEndian.PutUint32(packet[4:], h.Command)
	binary.LittleEndian.PutUint32(packet[8:], uint32(bodyLen))
	packet[12] = h.Channel
	packet[13] = h.Stream
	binary.LittleEndian.PutUint16(packet[14:], h.Sequence)
	binary.LittleEndian.PutUint16(packet[16:], h.ResponseCode)
	binary.LittleEndian.PutUint16(packet[18:], classOffset)
	binary.LittleEndian.PutUint32(packet[20:], uint32(len(ext)))
	copy(packet[24:], ext)
	copy(packet[24+len(ext):], payload)
	_, err := conn.Write(packet)
	return err
}
