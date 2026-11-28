package rtsp

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	goopus "github.com/skrashevich/go-opus"
)

func TestConsumerOpusRTP(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	_ = remote.SetReadDeadline(time.Now().Add(2 * time.Second))
	c := NewServer(local)
	c.mode, c.state, c.playOK = core.ModePassiveConsumer, StatePlay, true
	c.Medias = []*core.Media{{Kind: core.KindAudio, Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{{Name: core.CodecOpus}}}}
	defer c.Stop()
	input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}}}
	media := c.GetMedias()[0]
	_, codec := input.MatchMedia(media)
	if codec == nil {
		t.Fatal("no PCM input")
	}
	source := core.NewReceiver(input, input.Codecs[0])
	if err := c.AddTrack(media, codec, source); err != nil {
		t.Fatal(err)
	}
	source.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, Marker: true}, Payload: make([]byte, 160)})
	header := make([]byte, 4)
	if _, err := io.ReadFull(remote, header); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, binary.BigEndian.Uint16(header[2:]))
	if _, err := io.ReadFull(remote, data); err != nil {
		t.Fatal(err)
	}
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if header[0] != '$' || packet.PayloadType != 96 {
		t.Fatal("wrong RTSP framing")
	}
	d, err := goopus.NewDecoderWithOutput(48000, 1)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 960)
	n, err := d.DecodeToInt16(packet.Payload, samples)
	if err != nil || n != 960 {
		t.Fatalf("Opus output: %d samples, %v", len(samples), err)
	}
	c.Senders[0].Close()
	c.Senders[0].Wait()
}
