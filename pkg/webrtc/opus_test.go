package webrtc

import (
	"bytes"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
	goopus "github.com/skrashevich/go-opus"
)

type opusWriter chan *rtp.Packet

func (w opusWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	w <- &rtp.Packet{Header: *header, Payload: append([]byte(nil), payload...)}
	return len(payload), nil
}
func (w opusWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestConsumerEncodesOpus(t *testing.T) {
	pc, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	c := NewConn(pc)
	c.Mode = core.ModeActiveProducer
	t.Cleanup(func() { _ = c.Stop() })
	output := make(opusWriter, 2)
	local := NewTrack(core.KindAudio)
	local.writer = output
	tr, err := pc.AddTransceiverFromTrack(local)
	if err != nil {
		t.Fatal(err)
	}
	if err = tr.SetMid("audio"); err != nil {
		t.Fatal(err)
	}
	c.Medias = []*core.Media{{Kind: core.KindAudio, Direction: core.DirectionSendonly, ID: "audio",
		Codecs: []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: 111}}}}
	input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}}}
	media := c.GetMedias()[0]
	_, codec := input.MatchMedia(media)
	if codec == nil {
		t.Fatal("PCM input not advertised")
	}
	source := core.NewReceiver(input, input.Codecs[0])
	if err = c.AddTrack(media, codec, source); err != nil {
		t.Fatal(err)
	}
	if len(c.Medias[0].Codecs) != 1 {
		t.Fatal("local input aliases leaked into SDP codecs")
	}
	d, err := goopus.NewDecoderWithOutput(48000, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		source.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, Timestamp: uint32(i * 160)}, Payload: make([]byte, 160)})
		select {
		case packet := <-output:
			if packet.PayloadType != 111 || packet.Timestamp != uint32(i*960) {
				t.Fatalf("wrong RTP header: %+v", packet.Header)
			}
			samples := make([]int16, 1920)
			n, err := d.DecodeToInt16(packet.Payload, samples)
			if err != nil || n != 960 {
				t.Fatalf("Opus output: %d samples, %v", len(samples), err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no WebRTC Opus output")
		}
	}
}

func TestConsumerHomeKitOpus(t *testing.T) {
	pc, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	c := NewConn(pc)
	c.Mode = core.ModeActiveProducer
	t.Cleanup(func() { _ = c.Stop() })
	output := make(opusWriter, 4)
	local := NewTrack(core.KindAudio)
	local.writer = output
	tr, err := pc.AddTransceiverFromTrack(local)
	if err != nil {
		t.Fatal(err)
	}
	if err = tr.SetMid("audio"); err != nil {
		t.Fatal(err)
	}
	c.Medias = []*core.Media{{Kind: core.KindAudio, Direction: core.DirectionSendonly, ID: "audio", Codecs: []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: 111}}}}
	input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecOpus, ClockRate: 16000, Channels: 1, PayloadType: 99}}}
	media := c.GetMedias()[0]
	_, codec := input.MatchMedia(media)
	if codec == nil {
		t.Fatal("HomeKit Opus audio is not matched to browser Opus")
	}
	source := core.NewReceiver(input, input.Codecs[0])
	if err = c.AddTrack(media, codec, source); err != nil {
		t.Fatal(err)
	}
	// 20 ms packets, including source RTP timestamp wraparound.
	for i, ts := range []uint32{0xfffffe00, 0xffffff40, 0x80, 0x1c0} {
		packet := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(100 + i), Timestamp: ts, PayloadType: 99}, Payload: []byte{0xf8, 0xff, 0xfe}}
		source.WriteRTP(packet)
		select {
		case got := <-output:
			want := []uint32{0xfffffa00, 0xfffffdc0, 0x180, 0x540}[i]
			if got.Timestamp != want || got.PayloadType != 111 || got.SequenceNumber != uint16(i+1) || !bytes.Equal(got.Payload, packet.Payload) {
				t.Fatalf("wrong forwarded packet: %+v", got)
			}
			if packet.Timestamp != ts || packet.PayloadType != 99 {
				t.Fatal("shared input packet modified")
			}
		case <-time.After(time.Second):
			t.Fatal("no browser audio packet")
		}
	}
}
