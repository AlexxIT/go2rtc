package streams

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// deadProducer mimics the camera on 192.168.1.180: DESCRIBE succeeds, so
// GetProducer returns a producer, but the session dies right after PLAY.
type deadProducer struct {
	core.Connection
}

func (p *deadProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	track := core.NewReceiver(media, codec)
	p.Receivers = append(p.Receivers, track)
	return track, nil
}

func (p *deadProducer) Start() error {
	return errors.New("read tcp 192.168.1.114:1->192.168.1.180:554: i/o timeout")
}

func (p *deadProducer) Stop() error { return p.Connection.Stop() }

// TestReconnectBackoff guards the bug where retry was reset to 0 after every
// session, so a camera that accepted the connection but never sent data was
// re-dialed in a tight loop forever (retry=0 in every log line).
func TestReconnectBackoff(t *testing.T) {
	var dials atomic.Int32

	HandleFunc("dead", func(source string) (core.Producer, error) {
		dials.Add(1)
		return &deadProducer{Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "dead",
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}},
				},
			},
		}}, nil
	})

	prod := NewProducer("dead:never-streams")
	if err := prod.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := prod.GetTrack(prod.conn.GetMedias()[0], prod.conn.GetMedias()[0].Codecs[0]); err != nil {
		t.Fatalf("get track: %v", err)
	}

	dials.Store(0)
	prod.start()

	time.Sleep(3 * time.Second)
	prod.stop()

	n := dials.Load()
	t.Logf("re-dials in 3s: %d", n)

	// With the backoff working the first few retries are 1s apart, so ~3-4 dials.
	// Without it the loop spins as fast as Start() returns - thousands.
	if n > 10 {
		t.Errorf("re-dialed %d times in 3s - backoff is not applied", n)
	}
	if n == 0 {
		t.Error("producer never retried at all")
	}
}
