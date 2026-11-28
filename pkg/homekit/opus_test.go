package homekit

import (
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
	goopus "github.com/skrashevich/go-opus"
)

func TestProducerOpusTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var packets []*rtp.Packet
		h := timekeeper(&core.Codec{Name: core.CodecOpus, ClockRate: 16000}, func(p *rtp.Packet) { packets = append(packets, p) })
		for i := range 50 {
			h(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(i), Timestamp: 1000 + uint32(i)*320}, Payload: []byte{0x48, 0}})
			time.Sleep(20 * time.Millisecond)
		}
		if len(packets) != 50 {
			t.Fatalf("one second of HomeKit Opus lost packets: got %d, want 50", len(packets))
		}
		for i, p := range packets {
			if p.Timestamp != 1000+uint32(i)*320 || p.SequenceNumber != uint16(i) {
				t.Fatalf("packet %d: seq=%d timestamp=%d", i, p.SequenceNumber, p.Timestamp)
			}
		}
	})
}

func TestConsumerEncodesOpus(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	c := NewConsumer(local, nil)
	t.Cleanup(func() { _ = c.Stop() })
	c.audioSession = &srtp.Session{Local: &srtp.Endpoint{}}
	c.audioRTPTime = 20
	input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{{Name: core.CodecPCML, ClockRate: 16000, Channels: 1}}}
	media := c.GetMedias()[1]
	_, codec := input.MatchMedia(media)
	if codec == nil {
		t.Fatal("PCM input not advertised")
	}
	source := core.NewReceiver(input, input.Codecs[0])
	if err := c.AddTrack(media, codec, source); err != nil {
		t.Fatal(err)
	}
	if len(c.Medias[1].Codecs) != 1 || c.Senders[0].Codec.Name != core.CodecOpus {
		t.Fatal("wrong HomeKit media codec")
	}
	output := make(chan *rtp.Packet, 1)
	sender := c.Senders[0]
	original := sender.Handler
	sender.Handler = func(p *rtp.Packet) { original(p); output <- p }
	source.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2}, Payload: make([]byte, 640)})
	select {
	case packet := <-output:
		header := opus.UnmarshalHeader(packet.Payload)
		if header.FrameSize != 20*time.Millisecond || header.Frames != 1 {
			t.Fatalf("unsupported HomeKit packet: %+v", header)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no HomeKit Opus output")
	}
	sender.Close()
	sender.Wait()
}

func TestBackchannelClockAndLoss(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	c := NewConsumer(local, nil)
	defer c.Stop()
	c.audioSession = &srtp.Session{}
	media := c.Medias[2]
	source, err := c.GetTrack(media, media.Codecs[0])
	if err != nil {
		t.Fatal(err)
	}
	var packets []*core.Packet
	h, err := opus.TranscodeHandler(&core.Codec{Name: core.CodecPCML, ClockRate: 16000, Channels: 1}, source.Codec, func(p *core.Packet) { packets = append(packets, p) })
	if err != nil {
		t.Fatal(err)
	}
	source.AppendChild(&core.Node{Input: h})
	e, err := goopus.NewEncoder()
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1276)
	n, err := e.Encode(make([]byte, 1920), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []uint16{0, 2} {
		c.audioSession.OnReadRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: i, Timestamp: uint32(i) * 320}, Payload: data[:n]})
	}
	if len(packets) != 3 {
		t.Fatalf("got %d output packets, want packet/PLC/packet", len(packets))
	}
	for i, p := range packets {
		if p.Timestamp != uint32(i)*320 || len(p.Payload) != 640 {
			t.Fatalf("packet %d: ts=%d payload=%d", i, p.Timestamp, len(p.Payload))
		}
	}
}
