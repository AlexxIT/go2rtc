package baichuan

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestClientLoginEncryption(t *testing.T) {
	for _, tc := range []struct {
		code    uint16
		login   encryption
		ack     encryption
		session encryption
	}{
		{0xdd00, encryptionNone, encryptionNone, encryptionNone},
		{0xdd01, encryptionBC, encryptionBC, encryptionBC},
		{0xdd12, encryptionBC, encryptionBC, encryptionAES},
		{0xdd02, encryptionBC, encryptionAES, encryptionAES},
		{0xdd03, encryptionBC, encryptionAES, encryptionAES},
	} {
		t.Run(fmt.Sprintf("%04x", tc.code), func(t *testing.T) {
			clientConn, camera := net.Pipe()
			cfg, err := NewConfig("camera.local", "user", "password").normalized()
			if err != nil {
				t.Fatal(err)
			}
			client := newClient(context.Background(), cfg, clientConn)
			defer client.Close()
			defer camera.Close()
			_ = camera.SetDeadline(time.Now().Add(2 * time.Second))
			readyKey, ready, err := client.reserve(request{command: commandAbility})
			if err != nil {
				t.Fatal(err)
			}
			defer client.release(readyKey, ready)
			cameraErr := make(chan error, 1)
			go func() {
				nonceRequest, err := readFrame(camera, cfg.Limits)
				if err != nil {
					cameraErr <- err
					return
				}
				const nonce = "synthetic-nonce"
				state := cipherState{mode: tc.login}
				err = writeTestFrame(camera, header{
					Command: commandLogin, Sequence: nonceRequest.header.Sequence, ResponseCode: tc.code,
				}, nil, []byte("<body><Encryption><nonce>"+nonce+"</nonce></Encryption></body>"), state, false)
				if err == nil {
					var auth frame
					auth, err = readFrame(camera, cfg.Limits)
					if err == nil {
						var msg message
						msg, err = decodeFrame(auth, state, false)
						body, _ := buildLogin(cfg.username, cfg.password, nonce)
						if err == nil && string(msg.payload) != string(body) {
							err = fmt.Errorf("authentication did not use negotiated login cipher")
						}
					}
					if err == nil {
						state.setAESKey(deriveAESKey(nonce, cfg.password))
						state.mode = tc.ack
						err = writeTestFrame(camera, header{
							Command: commandLogin, Sequence: auth.header.Sequence, ResponseCode: 200,
						}, []byte("<body/>"), []byte("<body/>"), state, false)
					}
				}
				if err == nil {
					state.mode = tc.session
					err = writeTestFrame(camera, header{
						Command: commandAbility, Sequence: readyKey.sequence, ResponseCode: 200,
					}, nil, []byte("<body><Ready/></body>"), state, false)
				}
				if err == nil {
					err = writeTestFrame(camera, header{
						Command: commandLogin, Sequence: 65535, ResponseCode: 0xdd12,
					}, []byte("<body/>"), []byte("<body/>"), cipherState{mode: encryptionBC}, false)
				}
				if err == nil {
					var probe frame
					probe, err = readFrame(camera, cfg.Limits)
					if err == nil {
						var msg message
						msg, err = decodeFrame(probe, state, false)
						if err == nil && string(msg.payload) != "<body><Probe/></body>" {
							err = fmt.Errorf("control request did not use negotiated session cipher")
						}
					}
					if err == nil {
						err = writeTestFrame(camera, header{
							Command: probe.header.Command, Sequence: probe.header.Sequence, ResponseCode: 200,
						}, nil, []byte("<body><Reply/></body>"), state, false)
					}
				}
				cameraErr <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err = client.Login(ctx); err == nil {
				select {
				case msg := <-ready.response:
					if string(msg.payload) != "<body><Ready/></body>" {
						t.Error("control immediately after login used the previous cipher")
					}
				case <-ctx.Done():
					t.Fatal("control immediately after login was not delivered")
				}
				var msg message
				msg, err = client.roundTrip(ctx, request{
					command: commandDeviceInfo, class: classOffset, payload: []byte("<body><Probe/></body>"),
				})
				if err == nil && string(msg.payload) != "<body><Reply/></body>" {
					t.Error("control response did not use negotiated session cipher")
				}
			}
			if err != nil {
				t.Error(err)
			}
			if err = <-cameraErr; err != nil {
				t.Error(err)
			}
		})
	}
}

func TestClientPreviewAfterQueuedResponse(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			clientConn, camera := net.Pipe()
			cfg, err := NewConfig("camera.local", "user", "password").normalized()
			if err != nil {
				t.Fatal(err)
			}
			client := newClient(context.Background(), cfg, clientConn)
			defer client.Close()
			defer camera.Close()
			_ = camera.SetDeadline(time.Now().Add(time.Second))
			key, pending, err := client.reserve(request{command: commandPreview})
			if err != nil {
				t.Fatal(err)
			}
			defer client.release(key, pending)
			preview := &Preview{
				client: client, messages: make(chan message, 1), done: make(chan struct{}),
				queueLimit: int64(cfg.Limits.MaxMediaBuffer),
			}
			if err = client.addPreview(preview); err != nil {
				t.Fatal(err)
			}
			h := header{Command: commandPreview, Sequence: key.sequence, ResponseCode: 200}
			if err = writeTestFrame(camera, h, nil, nil, cipherState{}, false); err != nil {
				t.Fatal(err)
			}
			var extension []byte
			if explicit {
				extension = []byte(binaryExtensionXML)
			}
			// Keep the ACK unread, as when roundTrip's caller is descheduled.
			if err = writeTestFrame(camera, h, extension, infoFixture(), cipherState{}, true); err != nil {
				t.Fatal(err)
			}
			select {
			case msg := <-preview.messages:
				if !msg.binary || string(msg.payload) != string(infoFixture()) {
					t.Fatal("first media payload was not delivered intact")
				}
			case <-client.Done():
				t.Fatal(client.Err())
			case <-time.After(time.Second):
				t.Fatal("first media payload was not delivered")
			}
			if len(pending.response) != 1 || client.pending(key).response != nil {
				t.Fatal("ACK did not complete its reservation before media")
			}
		})
	}
}

func TestClientPreviewRejectsOtherCommands(t *testing.T) {
	clientConn, camera := net.Pipe()
	cfg, _ := NewConfig("camera.local", "user", "password").normalized()
	client := newClient(context.Background(), cfg, clientConn)
	defer client.Close()
	defer camera.Close()
	_ = camera.SetDeadline(time.Now().Add(time.Second))
	preview := &Preview{
		client: client, messages: make(chan message, 8), done: make(chan struct{}),
		queueLimit: int64(cfg.Limits.MaxMediaBuffer),
	}
	if err := client.addPreview(preview); err != nil {
		t.Fatal(err)
	}
	key, pending, err := client.reserve(request{command: commandDeviceInfo})
	if err != nil {
		t.Fatal(err)
	}
	defer client.release(key, pending)
	for _, command := range []uint32{commandTalkData, commandDeviceInfo, 999, commandPreview} {
		if err = writeTestFrame(camera, header{Command: command, ResponseCode: 200},
			[]byte(binaryExtensionXML), infoFixture(), cipherState{}, true); err != nil {
			t.Fatal(err)
		}
	}
	if err = writeTestFrame(camera, header{Command: commandDeviceInfo, Sequence: key.sequence, ResponseCode: 200},
		nil, []byte("<body/>"), cipherState{}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-pending.response:
		if string(msg.payload) != "<body/>" {
			t.Fatal("control response changed")
		}
	case <-time.After(time.Second):
		t.Fatal("control response not delivered")
	}
	if len(preview.messages) != 1 {
		t.Fatalf("preview received %d messages, want one", len(preview.messages))
	}
	if msg := <-preview.messages; msg.header.Command != commandPreview || string(msg.payload) != string(infoFixture()) {
		t.Fatal("preview payload changed")
	}
}
