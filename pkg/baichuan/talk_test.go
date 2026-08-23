package baichuan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClientTalk(t *testing.T) {
	clientConn, cameraConn := net.Pipe()
	cfg, err := NewConfig("camera.local", "admin", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, clientConn)
	defer client.Close()

	cameraErr := make(chan error, 1)
	go func() {
		cameraErr <- serveTalk(cameraConn, cfg)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	preview, err := client.StartPreview(ctx, 2, StreamMain)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		talk, err := client.StartTalk(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		packet, err := preview.Read(ctx)
		if err != nil || packet.Kind != MediaInfo {
			t.Fatalf("unexpected info packet: %+v, %v", packet, err)
		}
		format := talk.Format()
		if format.SampleRate != 16000 || format.SamplePrecision != 16 || format.SamplesPerBlock != 1016 {
			t.Fatalf("unexpected format: %+v", format)
		}
		encoder := ADPCMEncoder{}
		block, err := encoder.EncodeBlock(make([]int16, format.SamplesPerBlock))
		if err != nil {
			t.Fatal(err)
		}
		if err = talk.WriteBlock(ctx, block); err != nil {
			t.Fatal(err)
		}
		if err = talk.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err = talk.Close(ctx); err != nil {
			t.Fatalf("second close: %v", err)
		}
	}
	if err = preview.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-cameraErr; err != nil {
		t.Fatal(err)
	}
}

func TestTalkCloseAfterClientClose(t *testing.T) {
	clientConn, cameraConn := net.Pipe()
	defer cameraConn.Close()
	client := newClient(context.Background(), Config{}, clientConn)
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	talk := &Talk{client: client, closeDone: make(chan struct{})}
	if err := talk.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func serveTalk(conn net.Conn, cfg Config) error {
	defer conn.Close()
	aes, err := serveLogin(conn, cfg)
	if err != nil {
		return err
	}
	previewRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return err
	}
	preview, err := decodeFrame(previewRequest, aes, false)
	if err != nil {
		return err
	}
	if preview.header.Command != commandPreview || preview.header.Channel != 2 || preview.header.Stream != 0 {
		return fmt.Errorf("invalid preview request: %+v", preview.header)
	}
	if err = writeTestFrame(conn, header{
		Command: commandPreview, Channel: 2, Stream: 0, Sequence: preview.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, nil, nil, aes, false); err != nil {
		return err
	}
	for range 2 {
		abilityRequest, err := readFrame(conn, cfg.Limits)
		if err != nil {
			return err
		}
		ability, err := decodeFrame(abilityRequest, aes, false)
		if err != nil {
			return err
		}
		if ability.header.Command != commandTalkAbility || ability.header.Channel != 2 ||
			!strings.Contains(string(ability.extension), "<channelId>2</channelId>") {
			return fmt.Errorf("invalid talk ability request: %+v %s", ability.header, ability.extension)
		}
		body := []byte(`<?xml version="1.0" encoding="UTF-8"?><body><TalkAbility version="1.1"><duplexList><duplex>fullDuplex</duplex></duplexList><audioStreamModeList><audioStreamMode>speaker</audioStreamMode></audioStreamModeList><audioConfigList><audioConfig><audioType>adpcm</audioType><sampleRate>16000</sampleRate><samplePrecision>16</samplePrecision><lengthPerEncoder>1016</lengthPerEncoder><soundTrack>mono</soundTrack></audioConfig></audioConfigList></TalkAbility></body>`)
		if err = writeTestFrame(conn, header{
			Command: commandTalkAbility, Channel: 2, Sequence: ability.header.Sequence,
			ResponseCode: 200, Class: classOffset,
		}, nil, body, aes, false); err != nil {
			return err
		}

		configRequest, err := readFrame(conn, cfg.Limits)
		if err != nil {
			return err
		}
		config, err := decodeFrame(configRequest, aes, false)
		if err != nil {
			return err
		}
		if config.header.Command != commandTalkConfig ||
			!strings.Contains(string(config.payload), "<TalkConfig") ||
			!strings.Contains(string(config.payload), "<lengthPerEncoder>1016</lengthPerEncoder>") {
			return fmt.Errorf("invalid talk config: %+v %s", config.header, config.payload)
		}
		if err = writeTestFrame(conn, header{
			Command: commandPreview, Channel: 2, Stream: 0, Sequence: preview.header.Sequence,
			ResponseCode: 200, Class: classOffset,
		}, []byte("<body><binaryData>1</binaryData></body>"), infoFixture(), aes, true); err != nil {
			return err
		}
		if err = writeTestFrame(conn, header{
			Command: commandTalkConfig, Channel: 2, Sequence: config.header.Sequence,
			ResponseCode: 200, Class: classOffset,
		}, nil, nil, aes, false); err != nil {
			return err
		}

		dataFrame, err := readFrame(conn, cfg.Limits)
		if err != nil {
			return err
		}
		data, err := decodeFrame(dataFrame, aes, true)
		if err != nil {
			return err
		}
		if data.header.Command != commandTalkData || len(data.payload) != 528 ||
			binary.LittleEndian.Uint32(data.payload) != mediaADPCM ||
			binary.LittleEndian.Uint16(data.payload[4:6]) != 516 {
			return fmt.Errorf("invalid talk data: %+v payload=%d", data.header, len(data.payload))
		}

		stopRequest, err := readFrame(conn, cfg.Limits)
		if err != nil {
			return err
		}
		stop, err := decodeFrame(stopRequest, aes, false)
		if err != nil {
			return err
		}
		if stop.header.Command != commandStopTalk || stop.header.Channel != 2 {
			return fmt.Errorf("invalid stop talk: %+v", stop.header)
		}
		if err = writeTestFrame(conn, header{
			Command: commandStopTalk, Channel: 2, Sequence: stop.header.Sequence,
			ResponseCode: 200, Class: classOffset,
		}, nil, nil, aes, false); err != nil {
			return err
		}
	}
	stopPreviewRequest, err := readFrame(conn, cfg.Limits)
	if err != nil {
		return err
	}
	stopPreview, err := decodeFrame(stopPreviewRequest, aes, false)
	if err != nil {
		return err
	}
	if stopPreview.header.Command != commandStopPreview || stopPreview.header.Channel != 2 ||
		stopPreview.header.Stream != 0 {
		return fmt.Errorf("invalid stop preview: %+v", stopPreview.header)
	}
	return writeTestFrame(conn, header{
		Command: commandStopPreview, Channel: 2, Stream: 0, Sequence: stopPreview.header.Sequence,
		ResponseCode: 200, Class: classOffset,
	}, nil, nil, aes, false)
}

func TestSelectTalkConfigSkipsInvalidADPCMProfile(t *testing.T) {
	ability := &talkAbility{
		Duplexes: []talkDuplex{{Value: "fullDuplex"}},
		Modes:    []talkMode{{Value: "speaker"}},
	}
	for _, samples := range []uint32{1015, 1016} {
		ability.Configs = append(ability.Configs, struct {
			Value talkAudioConfig `xml:"audioConfig"`
		}{Value: talkAudioConfig{
			AudioType: "adpcm", SampleRate: 16000, SamplePrecision: 16,
			SamplesPerBlock: samples, SoundTrack: "mono",
		}})
	}
	config, err := selectTalkConfig(0, ability)
	if err != nil {
		t.Fatal(err)
	}
	if config.Audio.SamplesPerBlock != 1016 {
		t.Fatalf("selected unsafe profile: %+v", config.Audio)
	}
}

func TestDecodeTalkAbilityLimits(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(strings.Repeat(" ", maxTalkAbilityBody+1)),
		[]byte(`<body><TalkAbility>` + strings.Repeat(`<duplexList><duplex>fullDuplex</duplex></duplexList>`,
			maxTalkAbilityOptions+1) + `</TalkAbility></body>`),
		[]byte(`<body><TalkAbility version="` + strings.Repeat("v", maxTalkAbilityText+1) +
			`"></TalkAbility></body>`),
	} {
		if _, err := decodeTalkAbility(body); err == nil {
			t.Fatal("accepted oversized talk ability")
		}
	}
}

func TestSelectTalkConfigRejectsIncompleteAbility(t *testing.T) {
	valid := talkAudioConfig{
		AudioType: "adpcm", SampleRate: 16000, SamplePrecision: 16,
		SamplesPerBlock: 1016, SoundTrack: "mono",
	}
	for _, test := range []struct {
		name    string
		ability talkAbility
	}{
		{name: "duplex", ability: talkAbility{Modes: []talkMode{{Value: "speaker"}}}},
		{name: "mode", ability: talkAbility{Duplexes: []talkDuplex{{Value: "fullDuplex"}}}},
		{name: "ADPCM", ability: talkAbility{
			Duplexes: []talkDuplex{{Value: "fullDuplex"}}, Modes: []talkMode{{Value: "speaker"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name != "ADPCM" {
				test.ability.Configs = append(test.ability.Configs, struct {
					Value talkAudioConfig `xml:"audioConfig"`
				}{Value: valid})
			}
			if _, err := selectTalkConfig(0, &test.ability); err == nil {
				t.Fatal("accepted incomplete talk ability")
			}
		})
	}
}

func TestUnsupportedTalkStatus(t *testing.T) {
	for _, code := range []uint16{400, 404, 405, 501} {
		if !unsupportedTalkStatus(code) {
			t.Fatalf("status %d should mean unsupported", code)
		}
	}
	for _, code := range []uint16{401, 403, 422, 500, 503} {
		if unsupportedTalkStatus(code) {
			t.Fatalf("status %d should remain an operational error", code)
		}
	}
}

func TestADPCMEncoder(t *testing.T) {
	encoder := ADPCMEncoder{}
	block, err := encoder.EncodeBlock([]int16{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(block) != 5 || string(block) != "\x00\x00\x00\x00\x00" {
		t.Fatalf("unexpected silence block: %x", block)
	}
	for _, samples := range [][]int16{nil, {1}, {1, 2, 3}} {
		if _, err = encoder.EncodeBlock(samples); err == nil {
			t.Fatalf("accepted %d samples", len(samples))
		}
	}
}

func TestDecodeADPCMBlock(t *testing.T) {
	samples, err := DecodeADPCMBlock([]byte{0, 0, 0, 0, 0x10})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0] != 1 || samples[1] != 1 {
		t.Fatalf("unexpected samples: %v", samples)
	}
	for _, block := range [][]byte{nil, {0, 0, 0, 0}, {0, 0, 89, 0, 0}} {
		if _, err = DecodeADPCMBlock(block); err == nil {
			t.Fatalf("accepted invalid block: %x", block)
		}
	}
}

func FuzzDecodeADPCMBlock(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0x10})
	f.Add([]byte{0, 0, 89, 0, 0})
	f.Fuzz(func(t *testing.T, block []byte) {
		_, _ = DecodeADPCMBlock(block)
	})
}
