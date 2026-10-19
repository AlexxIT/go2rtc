package webrtc

import (
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
