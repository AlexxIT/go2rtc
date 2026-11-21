package streams

import (
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/probe"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// bcCamera is a camera connection with one video track and one talkback (backchannel), as an RTSP camera with an
// ONVIF backchannel is. Like a real camera it has a single speaker: every AddTrack sets the backchannel up on this
// connection, and it stays set up until the connection is gone.
type bcCamera struct {
	core.Connection

	video      *core.Receiver
	setups     atomic.Int32 // backchannel set up on this connection
	talkPkts   atomic.Int32 // packets that reached the speaker
	failed     chan struct{}
	stopped    chan struct{}
	stopOnce   sync.Once
	failedOnce sync.Once
}

func newBCCamera() *bcCamera {
	return &bcCamera{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "bctest",
			Medias: []*core.Media{
				{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecH264, ClockRate: 90000}}},
				{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: core.CodecPCMU, ClockRate: 8000}}},
			},
		},
		failed:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (c *bcCamera) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	c.video = core.NewReceiver(media, codec)
	return c.video, nil
}

func (c *bcCamera) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	c.setups.Add(1)
	sender := core.NewSender(media, codec)
	sender.Handler = func(*core.Packet) { c.talkPkts.Add(1) }
	sender.HandleRTP(track)
	return nil
}

// Start runs until the connection is stopped, or returns an error when the camera drops it.
func (c *bcCamera) Start() error {
	select {
	case <-c.stopped:
		return nil
	case <-c.failed:
		return core.ErrCantGetTrack
	}
}

func (c *bcCamera) Stop() error {
	c.stopOnce.Do(func() { close(c.stopped) })
	return nil
}

func (c *bcCamera) drop() { c.failedOnce.Do(func() { close(c.failed) }) }

func (c *bcCamera) isStopped() bool {
	select {
	case <-c.stopped:
		return true
	default:
		return false
	}
}

// bcSite registers a scheme whose every dial is a new bcCamera, and remembers them in order.
type bcSite struct {
	mu   sync.Mutex
	cams []*bcCamera
}

func newBCSite(scheme string) *bcSite {
	site := &bcSite{}
	HandleFunc(scheme, func(string) (core.Producer, error) {
		cam := newBCCamera()
		site.mu.Lock()
		site.cams = append(site.cams, cam)
		site.mu.Unlock()
		return cam, nil
	})
	return site
}

func (s *bcSite) dials() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cams)
}

func (s *bcSite) cam(i int) *bcCamera {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cams[i]
}

func (s *bcSite) last() *bcCamera {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cams[len(s.cams)-1]
}

// bcClient is a WebRTC-like client: it watches the video and, with a microphone, offers a track for the camera's
// speaker. Its Stop closes its tracks, as a real connection's Stop does.
type bcClient struct {
	core.Connection
	videoPkts atomic.Int32
}

func newBCClient(mic bool) *bcClient {
	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: core.CodecAny}}},
	}
	if mic {
		medias = append(medias, &core.Media{
			Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecPCMU, ClockRate: 8000}},
		})
	}
	return &bcClient{Connection: core.Connection{ID: core.NewID(), FormatName: "bcclient", Medias: medias}}
}

func (c *bcClient) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	track := core.NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, track)
	return track, nil
}

func (c *bcClient) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(*core.Packet) { c.videoPkts.Add(1) }
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *bcClient) Start() error { return nil }

func (c *bcClient) talk() { c.Receivers[0].WriteRTP(&rtp.Packet{Payload: make([]byte, 160)}) }

func microphoneProbe() *probe.Probe {
	return probe.Create("probe", url.Values{"video": {""}, "audio": {""}, "microphone": {""}})
}

const bcWait, bcTick = 2 * time.Second, 5 * time.Millisecond

// A stream with a reader (as Frigate's recorder is) on a fresh camera.
func newBCStream(t *testing.T, scheme string) (*Stream, *bcSite, *bcClient) {
	site := newBCSite(scheme)
	stream := NewStream(scheme + ":cam")
	reader := newBCClient(false)
	require.NoError(t, stream.AddConsumer(reader))
	require.Equal(t, 1, site.dials())
	return stream, site, reader
}

// A microphone probe (GET /api/streams?src=...&microphone, as Frigate's live page sends) must not take the camera's
// single speaker, during the probe or after it.
func TestBackchannelProbeDoesNotTakeSpeaker(t *testing.T) {
	stream, site, _ := newBCStream(t, "bcprobe")

	cons := microphoneProbe()
	require.NoError(t, stream.AddConsumer(cons))
	require.Zero(t, site.cam(0).setups.Load(), "the probe set up the camera's backchannel")

	stream.RemoveConsumer(cons)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, site.dials())
	require.Zero(t, site.last().setups.Load(), "the backchannel is held after the probe left")
	require.Empty(t, stream.producers[0].senders)
}

// After a talker closes normally, the speaker is free again: the camera connection that holds the backchannel is
// replaced by one without it, and the reader keeps getting video from the new connection.
func TestBackchannelReleasedWhenTalkerLeaves(t *testing.T) {
	stream, site, reader := newBCStream(t, "bcleave")

	talker := newBCClient(true)
	require.NoError(t, stream.AddConsumer(talker))
	require.EqualValues(t, 1, site.cam(0).setups.Load())
	talker.talk()
	require.Eventually(t, func() bool { return site.cam(0).talkPkts.Load() == 1 }, bcWait, bcTick)

	stream.RemoveConsumer(talker)

	require.Eventually(t, func() bool { return site.dials() == 2 }, bcWait, bcTick, "the camera was never re-dialled without the backchannel")
	require.Eventually(t, func() bool { return site.cam(0).isStopped() }, bcWait, bcTick, "the connection holding the backchannel is still open")
	require.Zero(t, site.cam(1).setups.Load(), "the new connection set the old backchannel up again")
	require.Empty(t, stream.producers[0].senders)

	before := reader.videoPkts.Load()
	site.cam(1).video.WriteRTP(&rtp.Packet{Payload: []byte{1}})
	require.Eventually(t, func() bool { return reader.videoPkts.Load() == before+1 }, bcWait, bcTick, "the reader lost the video")
}

// A talker that vanishes (the browser is killed, the network drops) is closed by its transport once the peer
// connection fails, and then removed (webrtc: PeerConnectionStateClosed => RemoveConsumer). The speaker is freed the
// same way, and a camera drop afterwards must not set the dead talker's backchannel up again.
func TestBackchannelReleasedAfterLostTalker(t *testing.T) {
	stream, site, _ := newBCStream(t, "bclost")

	talker := newBCClient(true)
	require.NoError(t, stream.AddConsumer(talker))
	require.EqualValues(t, 1, site.cam(0).setups.Load())

	_ = talker.Stop() // the transport gave up on the peer
	stream.RemoveConsumer(talker)

	require.Eventually(t, func() bool { return site.dials() == 2 }, bcWait, bcTick, "the camera was never re-dialled without the backchannel")
	require.Zero(t, site.cam(1).setups.Load())

	// the camera drops the connection: the regular reconnect must not bring the dead talker back
	site.cam(1).drop()
	require.Eventually(t, func() bool { return site.dials() == 3 }, bcWait, bcTick)
	require.Zero(t, site.cam(2).setups.Load(), "a stale backchannel was set up again after a camera drop")
}

// While someone talks, nothing else takes the speaker from them or re-dials their camera: not a probe, and not a
// second talker that comes and goes. Their audio keeps reaching the camera.
func TestBackchannelActiveTalkerKeepsSpeaker(t *testing.T) {
	stream, site, _ := newBCStream(t, "bcactive")

	talker := newBCClient(true)
	require.NoError(t, stream.AddConsumer(talker))
	require.EqualValues(t, 1, site.cam(0).setups.Load())

	cons := microphoneProbe()
	require.NoError(t, stream.AddConsumer(cons))
	stream.RemoveConsumer(cons)

	second := newBCClient(true)
	require.NoError(t, stream.AddConsumer(second))
	stream.RemoveConsumer(second)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, site.dials(), "the active talker's camera was re-dialled")
	require.False(t, site.cam(0).isStopped(), "the active talker's connection was stopped")
	require.Len(t, stream.producers[0].senders, 1)

	talker.talk()
	require.Eventually(t, func() bool { return site.cam(0).talkPkts.Load() == 1 }, bcWait, bcTick, "the active talker's audio no longer reaches the camera")
}
