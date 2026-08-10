package reolink

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
)

func TestParseURL(t *testing.T) {
	source, err := parseURL("reolink://admin:p%40ss@[fd00::1]:9001/sub?channel=2&backchannel=0")
	if err != nil {
		t.Fatal(err)
	}
	if source.config.Host != "fd00::1" || source.config.Port != 9001 || source.channel != 2 ||
		source.stream != baichuan.StreamSub || source.remote != "[fd00::1]:9001" || source.backchannel ||
		source.protocol != "tcp" {
		t.Fatalf("unexpected source: %+v", source)
	}
	text := fmt.Sprint(source.config)
	if strings.Contains(text, "admin") || strings.Contains(text, "p@ss") {
		t.Fatalf("credentials exposed by config: %s", text)
	}
}

func TestParseUIDURL(t *testing.T) {
	const uid = "AbC1234567890XYZ"
	source, err := parseURL("reolink://admin:secret@" + uid +
		"/sub?transport=uid&channel=1&backchannel=0&local=192.0.2.28&broadcast=198.51.100.255")
	if err != nil {
		t.Fatal(err)
	}
	if source.config.Host != "" || source.config.Port != 0 || source.channel != 1 ||
		source.stream != baichuan.StreamSub || source.remote != "uid" || source.backchannel ||
		source.config.UIDLocalAddr != "192.0.2.28" || source.config.UIDBroadcastAddr != "198.51.100.255" ||
		source.protocol != "udp" {
		t.Fatalf("unexpected UID source: %+v", source)
	}
	for _, value := range []string{fmt.Sprint(source), fmt.Sprint(source.config)} {
		if strings.Contains(value, uid) || strings.Contains(value, "secret") {
			t.Fatalf("UID source leaked: %s", value)
		}
	}
}

func TestParseURLRejectsInvalidSource(t *testing.T) {
	for _, source := range []string{
		"http://admin:pass@camera", "reolink://camera", "reolink://admin:pass@camera?channel=256",
		"reolink://admin:pass@camera?stream=fluent", "reolink://admin:pass@camera/main?stream=sub",
		"reolink://admin:pass@camera?unknown=1", "reolink://admin:pass@camera?backchannel=yes",
		"reolink://admin:pass@camera?channel=%zz",
		"reolink://admin:pass@camera:9000?transport=uid", "reolink://admin:pass@camera?transport=cloud",
		"reolink://admin:pass@camera?local=192.0.2.28",
		"reolink://admin:pass@camera?transport=uid&local=127.0.0.1",
		"reolink://admin:pass@camera?broadcast=198.51.100.255",
		"reolink://admin:pass@camera?transport=uid&broadcast=127.0.0.1",
		"reolink://admin:pass@camera?transport=uid&broadcast=224.0.0.1",
	} {
		if _, err := parseURL(source); err == nil {
			t.Fatalf("accepted invalid source: %s", source)
		}
	}
}

func TestParseURLErrorRedactsMalformedSource(t *testing.T) {
	const source = "reolink://sentinel-user:sentinel-pass@camera:%zz"
	_, err := parseURL(source)
	if err == nil || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("unsafe parse error: %v", err)
	}
}

func TestParseUIDErrorRegistersSecret(t *testing.T) {
	const uid = "MalformedUID1234"
	const source = "reolink://admin:pass@" + uid + "?transport=uid&unknown=1"
	if _, err := parseURL(source); err == nil {
		t.Fatal("accepted invalid UID source")
	}
	if redacted := creds.SecretString(source); strings.Contains(redacted, uid) {
		t.Fatalf("UID source leaked: %s", redacted)
	}
}

func TestMediaClock(t *testing.T) {
	clock := mediaClock{value: 100}
	if value, err := clock.next(0xfffffff0, 90000); err != nil || value != 100 {
		t.Fatalf("unexpected initial value: %d, %v", value, err)
	}
	if value, err := clock.next(0x0000c340, 90000); err != nil || value != 4600 {
		t.Fatalf("unexpected wrapped value: %d, %v", value, err)
	}
	if value, err := clock.next(0x0000c340, 90000); err != nil || value != 4600 {
		t.Fatalf("unexpected duplicate value: %d, %v", value, err)
	}
}

func TestMediaClockRejectsDiscontinuity(t *testing.T) {
	for _, timestamps := range [][2]uint32{
		{1_000_000, 900_000},
		{1_000_000, 1_000_000 + maxTimestampStep + 1},
	} {
		clock := mediaClock{}
		if _, err := clock.next(timestamps[0], 90000); err != nil {
			t.Fatal(err)
		}
		if _, err := clock.next(timestamps[1], 90000); err == nil {
			t.Fatalf("accepted timestamp discontinuity %v", timestamps)
		}
	}
}

func TestNextTimestampEpoch(t *testing.T) {
	for _, test := range []struct {
		name          string
		now, previous uint32
		want          uint32
	}{
		{name: "Forward", now: 200, previous: 100, want: 200},
		{name: "Backward", now: 90, previous: 100, want: 101},
		{name: "Duplicate", now: 100, previous: 100, want: 101},
		{name: "Wrap", now: 20, previous: 0xfffffff0, want: 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := nextTimestampEpoch(test.now, test.previous, true); actual != test.want {
				t.Fatalf("unexpected epoch: %d != %d", actual, test.want)
			}
		})
	}
}

func TestAudioPayload(t *testing.T) {
	header := []byte{0xff, 0xf1, 0x60, 0x40, 0x00, 0x1f, 0xfc}
	data := append(header, []byte{1, 2, 3}...)
	aac.WriteADTSSize(data, uint16(len(data)))
	payload, samples, err := audioPayload(baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "\x01\x02\x03" || samples != 1024 {
		t.Fatalf("unexpected AAC packet: %x %d", payload, samples)
	}
}

func TestADPCMAudioPayload(t *testing.T) {
	payload, samples, err := audioPayload(baichuan.MediaPacket{
		Kind: baichuan.MediaADPCM, Data: []byte{0, 0, 0, 0, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if samples != 2 || len(payload) != 2 || payload[0] != 0xd5 || payload[1] != 0xd5 {
		t.Fatalf("unexpected PCMA payload: %x samples=%d", payload, samples)
	}
}

func TestAddAudioRejectsInvalidAACConfig(t *testing.T) {
	base := []byte{0xff, 0xf1, 0x60, 0x40, 0, 0, 0xfc}
	reservedRate := append([]byte(nil), base...)
	reservedRate[2] = reservedRate[2]&0xc3 | 0x3c
	noChannels := append([]byte(nil), base...)
	noChannels[2] &^= 1
	noChannels[3] &^= 0xc0
	for _, data := range [][]byte{reservedRate, noChannels} {
		pipeline := &mediaPipeline{}
		if err := pipeline.configureAudio(baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: data}); err == nil {
			t.Fatalf("accepted invalid AAC config: %x", data)
		}
	}
}

func TestWritePacketRejectsAACConfigChange(t *testing.T) {
	data := []byte{0xff, 0xf1, 0x60, 0x40, 0, 0, 0xfc}
	pipeline := &mediaPipeline{}
	if err := pipeline.configureAudio(baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: data}); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), data...)
	changed[2] += 4
	if _, err := pipeline.frame(baichuan.MediaPacket{Kind: baichuan.MediaAAC, Data: changed}); err == nil ||
		!strings.Contains(err.Error(), "configuration changed") {
		t.Fatalf("unexpected AAC configuration error: %v", err)
	}
}

func TestRetainProbePacketKeepsLatestGOP(t *testing.T) {
	probe := &mediaProbe{}
	for _, packet := range []baichuan.MediaPacket{
		{Kind: baichuan.MediaVideoI, Data: make([]byte, 10)},
		{Kind: baichuan.MediaVideoP, Data: make([]byte, 5)},
		{Kind: baichuan.MediaVideoI, Data: make([]byte, 7)},
	} {
		if err := probe.retain(packet); err != nil {
			t.Fatal(err)
		}
	}
	if len(probe.packets) != 1 || probe.bytes != 7 || probe.packets[0].Kind != baichuan.MediaVideoI {
		t.Fatalf("unexpected retained probe: packets=%d bytes=%d", len(probe.packets), probe.bytes)
	}
}

func TestRetainProbePacketOwnsPayload(t *testing.T) {
	backing := make([]byte, 1024)
	packet := baichuan.MediaPacket{Kind: baichuan.MediaVideoP, Data: backing[len(backing)-1:]}
	packet.Data[0] = 1
	probe := &mediaProbe{}
	if err := probe.retain(packet); err != nil {
		t.Fatal(err)
	}
	packet.Data[0] = 2
	if probe.packets[0].Data[0] != 1 {
		t.Fatal("probe retained the source payload allocation")
	}
}

func TestRetainProbePacketLimit(t *testing.T) {
	probe := &mediaProbe{bytes: maxProbeBytes}
	if err := probe.retain(baichuan.MediaPacket{Kind: baichuan.MediaVideoP, Data: []byte{1}}); err == nil {
		t.Fatal("accepted probe packet beyond memory limit")
	}
}

func TestRetainProbePacketCountLimitAndRelease(t *testing.T) {
	probe := &mediaProbe{}
	for range maxProbePackets {
		if err := probe.retain(baichuan.MediaPacket{Kind: baichuan.MediaInfo}); err != nil {
			t.Fatal(err)
		}
	}
	if err := probe.retain(baichuan.MediaPacket{Kind: baichuan.MediaInfo}); err == nil {
		t.Fatal("accepted probe packet beyond count limit")
	}
	probe.release()
	if probe.packets != nil || probe.bytes != 0 {
		t.Fatalf("probe retained data after release: packets=%d bytes=%d", len(probe.packets), probe.bytes)
	}
}

func TestBackchannelRejectsOversizedPacketBeforeDial(t *testing.T) {
	p := &Producer{ctx: context.Background()}
	b := &backchannel{producer: p, format: baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 1016}}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(make([]byte, 2033)); err == nil || !strings.Contains(err.Error(), "sample limit") {
		t.Fatalf("unexpected oversized packet error: %v", err)
	}
}
