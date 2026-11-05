package baichuan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseCapabilities(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><body><AbilityInfo>` +
		`<system><subModule><abilityValue>version_ro, reboot_rw, ignored</abilityValue></subModule></system>` +
		`<streaming><subModule><channelId>1</channelId><abilityValue>live_ro</abilityValue></subModule>` +
		`<subModule><channelId>0</channelId><abilityValue>live_rw, audio_ro, version_rw</abilityValue></subModule></streaming>` +
		`</AbilityInfo></body>`)
	value, err := parseCapabilities(body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.ObservedChannels, []uint8{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("channels = %v, want %v", got, want)
	}
	clone := value.clone()
	clone.ObservedChannels[0] = 2
	if value.ObservedChannels[0] == 2 {
		t.Fatal("clone shares capability storage")
	}
}

func TestParseCapabilitiesRejectsLimits(t *testing.T) {
	if _, err := parseCapabilities(make([]byte, maxCapabilityBody+1)); err == nil {
		t.Fatal("accepted oversized response")
	}
	if _, err := parseCapabilities([]byte(`<body/>`)); err == nil {
		t.Fatal("accepted response without AbilityInfo")
	}
}

func TestClientDiscoveryCachesResponses(t *testing.T) {
	clientConn, cameraConn := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, clientConn)
	t.Cleanup(func() { _ = client.Close() })
	cameraErr := make(chan error, 1)
	go func() {
		defer cameraConn.Close()
		aes, err := serveLogin(cameraConn, cfg)
		if err != nil {
			cameraErr <- err
			return
		}
		requests := make(map[uint32]message, 2)
		for range 2 {
			frame, err := readFrame(cameraConn, cfg.Limits)
			if err != nil {
				cameraErr <- err
				return
			}
			message, err := decodeFrame(frame, aes, false)
			if err != nil {
				cameraErr <- err
				return
			}
			if message.header.Command != commandDeviceInfo && message.header.Command != commandAbility {
				cameraErr <- fmt.Errorf("unexpected command %d", message.header.Command)
				return
			}
			if _, ok := requests[message.header.Command]; ok {
				cameraErr <- fmt.Errorf("duplicate command %d", message.header.Command)
				return
			}
			if message.header.Command == commandAbility &&
				(!strings.Contains(string(message.extension), "<userName>admin</userName>") ||
					!strings.Contains(string(message.extension), "<token>")) {
				cameraErr <- fmt.Errorf("invalid capability extension")
				return
			}
			requests[message.header.Command] = message
		}
		for _, command := range []uint32{commandAbility, commandDeviceInfo} {
			message := requests[command]
			var payload string
			if command == commandAbility {
				payload = `<body><AbilityInfo><system><subModule><channelId>0</channelId><abilityValue>version_ro</abilityValue></subModule></system></AbilityInfo></body>`
			} else {
				payload = `<body><VersionInfo><type>E1 Zoom</type><itemNo>E340</itemNo></VersionInfo></body>`
			}
			if err = writeTestFrame(cameraConn, header{
				Command: command, Sequence: message.header.Sequence,
				ResponseCode: 200, Class: classOffset,
			}, nil, []byte(payload), aes, false); err != nil {
				cameraErr <- err
				return
			}
		}
		cameraErr <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type deviceResult struct {
		value DeviceInfo
		err   error
	}
	type capabilityResult struct {
		value Capabilities
		err   error
	}
	deviceDone := make(chan deviceResult, 1)
	capabilityDone := make(chan capabilityResult, 1)
	go func() {
		value, err := client.DeviceInfo(ctx)
		deviceDone <- deviceResult{value, err}
	}()
	go func() {
		value, err := client.Capabilities(ctx, 0)
		capabilityDone <- capabilityResult{value, err}
	}()
	device := <-deviceDone
	if device.err != nil || device.value.DisplayModel() != "E340" {
		t.Fatalf("device = %+v, %v", device.value, device.err)
	}
	capabilities := <-capabilityDone
	if capabilities.err != nil || !slices.Equal(capabilities.value.ObservedChannels, []uint8{0}) {
		t.Fatalf("capabilities = %+v, %v", capabilities.value, capabilities.err)
	}
	if cached, err := client.DeviceInfo(ctx); err != nil || cached != device.value {
		t.Fatalf("cached device = %+v, %v", cached, err)
	}
	capabilities.value.ObservedChannels[0] = 1
	if cached, err := client.Capabilities(ctx, 0); err != nil ||
		!slices.Equal(cached.ObservedChannels, []uint8{0}) {
		t.Fatalf("cached capabilities = %+v, %v", cached, err)
	}
	if err = <-cameraErr; err != nil {
		t.Fatal(err)
	}
}

func TestClientDiscoveryFailuresDoNotClosePreview(t *testing.T) {
	clientConn, cameraConn := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, clientConn)
	t.Cleanup(func() { _ = client.Close() })
	cameraErr := make(chan error, 1)
	go func() {
		defer cameraConn.Close()
		aes, err := serveLogin(cameraConn, cfg)
		if err != nil {
			cameraErr <- err
			return
		}
		requests := make(map[uint32]message, 2)
		for range 2 {
			frame, err := readFrame(cameraConn, cfg.Limits)
			if err != nil {
				cameraErr <- err
				return
			}
			message, err := decodeFrame(frame, aes, false)
			if err != nil {
				cameraErr <- err
				return
			}
			requests[message.header.Command] = message
		}
		device, ok := requests[commandDeviceInfo]
		if !ok || requests[commandAbility].header.Command != commandAbility {
			cameraErr <- fmt.Errorf("missing discovery requests")
			return
		}
		if err = writeTestFrame(cameraConn, header{
			Command: commandDeviceInfo, Sequence: device.header.Sequence,
			ResponseCode: 500, Class: classOffset,
		}, nil, nil, aes, false); err != nil {
			cameraErr <- err
			return
		}
		cameraErr <- servePreviewSession(cameraConn, cfg, aes, previewRoute{stream: StreamMain})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	deviceDone := make(chan error, 1)
	capabilityDone := make(chan error, 1)
	go func() {
		_, err := client.DeviceInfo(ctx)
		deviceDone <- err
	}()
	go func() {
		_, err := client.Capabilities(ctx, 0)
		capabilityDone <- err
	}()
	var status *StatusError
	if err = <-deviceDone; !errors.As(err, &status) || status.Code != 500 {
		t.Fatalf("unexpected device failure: %v", err)
	}
	if err = <-capabilityDone; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected capability failure: %v", err)
	}

	previewCtx, previewCancel := context.WithTimeout(context.Background(), time.Second)
	defer previewCancel()
	preview, err := client.StartPreview(previewCtx, 0, StreamMain)
	if err != nil {
		t.Fatalf("preview after discovery failure: %v; camera: %v", err, <-cameraErr)
	}
	for _, kind := range []MediaKind{MediaInfo, MediaVideoI} {
		packet, err := preview.Read(previewCtx)
		if err != nil || packet.Kind != kind {
			t.Fatalf("unexpected preview packet: %+v, %v", packet, err)
		}
	}
	if err = preview.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-cameraErr; err != nil {
		t.Fatal(err)
	}
}
