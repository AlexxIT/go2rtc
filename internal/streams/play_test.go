package streams

import (
	"errors"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

type playProducer struct {
	medias []*core.Media
}

func (p *playProducer) GetMedias() []*core.Media { return p.medias }

func (p *playProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return core.NewReceiver(media, codec), nil
}

func (p *playProducer) Start() error { return nil }
func (p *playProducer) Stop() error  { return nil }

type playConsumer struct {
	playProducer
	err    error
	tracks int
}

// CanMix, so a new playback is added to the running session
func (c *playConsumer) CanMix() bool { return true }

func (c *playConsumer) AddTrack(*core.Media, *core.Codec, *core.Receiver) error {
	if c.err != nil {
		return c.err
	}
	c.tracks++
	return nil
}

func pcmaMedia(direction string) []*core.Media {
	return []*core.Media{{
		Kind:      core.KindAudio,
		Direction: direction,
		Codecs:    []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}},
	}}
}

func TestPlayRunningSessionNoMatch(t *testing.T) {
	// a new connection to the camera would accept the playback
	var dialed *playConsumer
	HandleFunc("playtest", func(string) (core.Producer, error) {
		dialed = &playConsumer{playProducer: playProducer{medias: pcmaMedia(core.DirectionRecvonly)}}
		return dialed, nil
	})

	// the running session can't take it
	cons := &playConsumer{
		playProducer: playProducer{medias: pcmaMedia(core.DirectionRecvonly)},
		err:          errors.New("no"),
	}
	ps := &playSession{dst: cons, cons: cons, done: make(chan struct{})}

	s := NewStream("playtest://camera")
	s.play = ps

	src := &playProducer{medias: pcmaMedia(core.DirectionRecvonly)}

	require.Error(t, s.Play(src))
	require.Same(t, ps, s.play)
	require.Nil(t, dialed, "no second connection to the camera")
}
