package webrtc

import (
	"sync"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestActiveProducerTrackReuse(t *testing.T) {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	offered := &core.Codec{Name: core.CodecH264, ClockRate: 90000, FmtpLine: "profile-level-id=42e01f", PayloadType: 96}
	remote := offered.Clone()
	remote.PayloadType = 97

	for _, codecs := range [][2]*core.Codec{{offered, remote}, {remote, offered}} {
		conn := &Conn{Mode: core.ModeActiveProducer}
		before := *codecs[0]
		first := mustGetTrack(t, conn, media, codecs[0])
		second := mustGetTrack(t, conn, media, codecs[1])
		if first != second {
			t.Fatal("payload type created a second receiver")
		}
		if first.Codec != codecs[0] || *first.Codec != before {
			t.Fatal("receiver codec changed")
		}
		if len(conn.Receivers) != 1 {
			t.Fatalf("wrong receiver count: %d", len(conn.Receivers))
		}
	}
}

func TestActiveProducerConcurrentTrackReuse(t *testing.T) {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	codecs := []*core.Codec{
		{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96},
		{Name: core.CodecH264, ClockRate: 90000, PayloadType: 97},
	}
	conn := &Conn{Mode: core.ModeActiveProducer}

	const workers = 100
	tracks := make([]*core.Receiver, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(workers)
	for i := range workers {
		go func() {
			defer group.Done()
			<-start
			var err error
			tracks[i], err = conn.GetTrack(media, codecs[i%len(codecs)])
			if err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	group.Wait()
	for _, track := range tracks[1:] {
		if track != tracks[0] {
			t.Fatal("concurrent call created a second receiver")
		}
	}
}

func TestActiveProducerKeepsDistinctTracks(t *testing.T) {
	media := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	codec := &core.Codec{Name: core.CodecH264, ClockRate: 90000, FmtpLine: "profile-level-id=42e01f", PayloadType: 96}
	conn := &Conn{Mode: core.ModeActiveProducer}
	track := mustGetTrack(t, conn, media, codec)

	otherMedia := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	otherCodec := codec.Clone()
	otherCodec.PayloadType = 97
	if track == mustGetTrack(t, conn, otherMedia, otherCodec) {
		t.Fatal("different media shared a receiver")
	}

	for _, other := range []*core.Codec{
		{Name: core.CodecH265, ClockRate: 90000},
		{Name: core.CodecH264, ClockRate: 48000, FmtpLine: codec.FmtpLine},
		{Name: core.CodecH264, ClockRate: 90000, Channels: 2, FmtpLine: codec.FmtpLine},
		{Name: core.CodecH264, ClockRate: 90000, FmtpLine: "profile-level-id=640032"},
	} {
		if track == mustGetTrack(t, conn, media, other) {
			t.Fatal("different codecs shared a receiver")
		}
	}

	conn = &Conn{Mode: core.ModePassiveProducer}
	if mustGetTrack(t, conn, media, codec) == mustGetTrack(t, conn, media, otherCodec) {
		t.Fatal("passive producer reused a receiver")
	}
}

func mustGetTrack(t *testing.T, conn *Conn, media *core.Media, codec *core.Codec) *core.Receiver {
	t.Helper()
	track, err := conn.GetTrack(media, codec)
	if err != nil {
		t.Fatal(err)
	}
	return track
}
