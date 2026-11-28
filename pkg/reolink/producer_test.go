package reolink

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
)

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

func timestampAfter(value, previous uint32) bool {
	delta := value - previous
	return delta != 0 && delta < 1<<31
}

func TestNextTimestampEpoch(t *testing.T) {
	for _, test := range []struct {
		name          string
		now, previous uint64
		want          uint64
	}{
		{name: "Forward", now: 200, previous: 100, want: 200},
		{name: "Backward", now: 90, previous: 100, want: 101},
		{name: "Duplicate", now: 100, previous: 100, want: 101},
		{name: "Wrap", now: 1<<32 + 20, previous: 0xfffffff0, want: 1<<32 + 20},
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

func TestRetainProbePacket(t *testing.T) {
	for _, test := range []struct {
		name    string
		packets []baichuan.MediaPacket
	}{
		{"latest_GOP", []baichuan.MediaPacket{
			{Kind: baichuan.MediaVideoI, Data: make([]byte, 10)},
			{Kind: baichuan.MediaVideoP, Data: make([]byte, 5)},
			{Kind: baichuan.MediaVideoI, Data: make([]byte, 7)},
		}},
		{"owned_payload", []baichuan.MediaPacket{
			{Kind: baichuan.MediaVideoP, Data: make([]byte, 1024)[1023:]},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			last := test.packets[len(test.packets)-1]
			last.Data[0] = 1
			probe := &mediaProbe{}
			for i, packet := range test.packets {
				if err := probe.retain(packet); err != nil {
					t.Fatalf("retain packet %d: %v", i, err)
				}
			}
			last.Data[0] = 2
			if len(probe.packets) != 1 || probe.bytes != len(last.Data) {
				t.Fatalf("retained %d packets and %d bytes, want 1 packet and %d bytes", len(probe.packets), probe.bytes, len(last.Data))
			}
			got := probe.packets[0]
			if got.Kind != last.Kind || len(got.Data) != len(last.Data) || got.Data[0] != 1 {
				t.Fatalf("retained kind=%v payload=%x, want kind=%v with original payload", got.Kind, got.Data, last.Kind)
			}
		})
	}
}

func TestRetainProbePacketLimits(t *testing.T) {
	for _, test := range []struct {
		limit          string
		bytes, packets int
		packet         baichuan.MediaPacket
	}{
		{"bytes", maxProbeBytes - 1, 1, baichuan.MediaPacket{Kind: baichuan.MediaVideoP, Data: []byte{1}}},
		{"packets", 0, maxProbePackets, baichuan.MediaPacket{Kind: baichuan.MediaInfo}},
	} {
		t.Run(test.limit, func(t *testing.T) {
			probe := &mediaProbe{bytes: test.bytes}
			for i := range test.packets {
				if err := probe.retain(test.packet); err != nil {
					t.Fatalf("retain packet %d before %s limit: %v", i, test.limit, err)
				}
			}
			if err := probe.retain(test.packet); err == nil || !strings.Contains(err.Error(), test.limit) {
				t.Fatalf("error = %v, want %s limit", err, test.limit)
			}
			probe.release()
			if probe.packets != nil || probe.bytes != 0 {
				t.Fatalf("probe retained data after release: packets=%d bytes=%d", len(probe.packets), probe.bytes)
			}
		})
	}
}

func TestBackchannelRejectsOversizedPacketBeforeDial(t *testing.T) {
	p := &Producer{stopCtx: context.Background()}
	b := &backchannel{producer: p, format: baichuan.TalkFormat{SampleRate: 16000, SamplesPerBlock: 1016}}
	if err := b.setCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(make([]byte, maxTalkPacketSamples*8000/16000+1)); err == nil || !strings.Contains(err.Error(), "sample limit") {
		t.Fatalf("unexpected oversized packet error: %v", err)
	}
}
