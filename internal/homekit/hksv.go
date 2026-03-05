package homekit

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/pion/rtp"
)

// hksvSession manages the HDS DataStream connection for HKSV recording
type hksvSession struct {
	server  *server
	hapConn *hap.Conn
	hdsConn *hds.Conn
	session *hds.Session

	mu       sync.Mutex
	consumer *hksvConsumer
}

func newHKSVSession(srv *server, hapConn *hap.Conn, hdsConn *hds.Conn) *hksvSession {
	session := hds.NewSession(hdsConn)
	hs := &hksvSession{
		server:  srv,
		hapConn: hapConn,
		hdsConn: hdsConn,
		session: session,
	}
	session.OnDataSendOpen = hs.handleOpen
	session.OnDataSendClose = hs.handleClose
	return hs
}

func (hs *hksvSession) Run() error {
	return hs.session.Run()
}

func (hs *hksvSession) Close() {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.consumer != nil {
		hs.stopRecording()
	}
	_ = hs.session.Close()
}

func (hs *hksvSession) handleOpen(streamID int) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	log.Debug().Str("stream", hs.server.stream).Int("streamID", streamID).Msg("[homekit] HKSV dataSend open")

	if hs.consumer != nil {
		hs.stopRecording()
	}

	// Try to use the pre-started consumer from pair-verify
	consumer := hs.server.takePreparedConsumer()
	if consumer != nil {
		log.Debug().Str("stream", hs.server.stream).Msg("[homekit] HKSV using prepared consumer")
		hs.consumer = consumer
		hs.server.AddConn(consumer)

		// Activate: set the HDS session and send init + start streaming
		if err := consumer.activate(hs.session, streamID); err != nil {
			log.Error().Err(err).Str("stream", hs.server.stream).Msg("[homekit] HKSV activate failed")
			hs.stopRecording()
			return nil
		}
		return nil
	}

	// Fallback: create new consumer (will be slow ~3s)
	log.Debug().Str("stream", hs.server.stream).Msg("[homekit] HKSV no prepared consumer, creating new")
	consumer = newHKSVConsumer()

	stream := streams.Get(hs.server.stream)
	if err := stream.AddConsumer(consumer); err != nil {
		log.Error().Err(err).Str("stream", hs.server.stream).Msg("[homekit] HKSV add consumer failed")
		return nil
	}

	hs.consumer = consumer
	hs.server.AddConn(consumer)

	go func() {
		if err := consumer.activate(hs.session, streamID); err != nil {
			log.Error().Err(err).Str("stream", hs.server.stream).Msg("[homekit] HKSV activate failed")
		}
	}()

	return nil
}

func (hs *hksvSession) handleClose(streamID int) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	log.Debug().Str("stream", hs.server.stream).Int("streamID", streamID).Msg("[homekit] HKSV dataSend close")

	if hs.consumer != nil {
		hs.stopRecording()
	}
	return nil
}

func (hs *hksvSession) stopRecording() {
	consumer := hs.consumer
	hs.consumer = nil

	stream := streams.Get(hs.server.stream)
	stream.RemoveConsumer(consumer)
	_ = consumer.Stop()
	hs.server.DelConn(consumer)
}

// hksvConsumer implements core.Consumer, generates fMP4 and sends over HDS.
// It can be pre-started without an HDS session, buffering init data until activated.
type hksvConsumer struct {
	core.Connection
	muxer *mp4.Muxer
	mu    sync.Mutex
	done  chan struct{}

	// Set by activate() when HDS session is available
	session  *hds.Session
	streamID int
	seqNum   int
	active   bool
	start    bool // waiting for first keyframe

	// GOP buffer - accumulate moof+mdat pairs, flush on next keyframe
	fragBuf []byte

	// Pre-built init segment (built when tracks connect)
	initData []byte
	initErr  error
	initDone chan struct{} // closed when init is ready
}

func newHKSVConsumer() *hksvConsumer {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecAAC},
			},
		},
	}
	return &hksvConsumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "hksv",
			Protocol:   "hds",
			Medias:     medias,
		},
		muxer:    &mp4.Muxer{},
		done:     make(chan struct{}),
		initDone: make(chan struct{}),
	}
}

func (c *hksvConsumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := byte(len(c.Senders))

	log.Debug().Str("codec", track.Codec.Name).Uint8("trackID", trackID).Msg("[homekit] HKSV AddTrack")

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		handler.Handler = func(packet *rtp.Packet) {
			c.mu.Lock()
			if !c.active {
				c.mu.Unlock()
				return
			}
			if !c.start {
				if !h264.IsKeyframe(packet.Payload) {
					c.mu.Unlock()
					return
				}
				c.start = true
				log.Debug().Int("payloadLen", len(packet.Payload)).Msg("[homekit] HKSV first keyframe")
			} else if h264.IsKeyframe(packet.Payload) && len(c.fragBuf) > 0 {
				// New keyframe = flush previous GOP as one mediaFragment
				c.flushFragment()
			}

			b := c.muxer.GetPayload(trackID, packet)
			c.fragBuf = append(c.fragBuf, b...)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecAAC:
		handler.Handler = func(packet *rtp.Packet) {
			c.mu.Lock()
			if !c.active || !c.start {
				c.mu.Unlock()
				return
			}

			b := c.muxer.GetPayload(trackID, packet)
			c.fragBuf = append(c.fragBuf, b...)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = aac.RTPDepay(handler.Handler)
		}

	default:
		return nil // skip unsupported codecs
	}

	c.muxer.AddTrack(codec)
	handler.HandleRTP(track)
	c.Senders = append(c.Senders, handler)

	// Build init segment when all expected tracks are ready (video + audio)
	select {
	case <-c.initDone:
		// already built
	default:
		if len(c.Senders) >= len(c.Medias) {
			initData, err := c.muxer.GetInit()
			c.initData = initData
			c.initErr = err
			close(c.initDone)
			if err != nil {
				log.Error().Err(err).Msg("[homekit] HKSV GetInit failed")
			} else {
				log.Debug().Int("initSize", len(initData)).Int("tracks", len(c.Senders)).Msg("[homekit] HKSV init segment ready")
			}
		}
	}

	return nil
}

// activate is called when the HDS session is ready (dataSend.open).
// It sends the pre-built init segment and starts streaming.
func (c *hksvConsumer) activate(session *hds.Session, streamID int) error {
	// Wait for init to be ready (should already be done if consumer was pre-started)
	select {
	case <-c.initDone:
	case <-time.After(5 * time.Second):
		return io.ErrClosedPipe
	}

	if c.initErr != nil {
		return c.initErr
	}

	log.Debug().Int("initSize", len(c.initData)).Msg("[homekit] HKSV sending init segment")

	if err := session.SendMediaInit(streamID, c.initData); err != nil {
		return err
	}

	log.Debug().Msg("[homekit] HKSV init segment sent OK")

	// Enable live streaming (seqNum=2 because init used seqNum=1)
	c.mu.Lock()
	c.session = session
	c.streamID = streamID
	c.seqNum = 2
	c.active = true
	c.mu.Unlock()

	return nil
}

// flushFragment sends the accumulated GOP buffer as a single mediaFragment.
// Must be called while holding c.mu.
func (c *hksvConsumer) flushFragment() {
	fragment := c.fragBuf
	c.fragBuf = make([]byte, 0, len(fragment))

	log.Debug().Int("fragSize", len(fragment)).Int("seq", c.seqNum).Msg("[homekit] HKSV flush fragment")

	if err := c.session.SendMediaFragment(c.streamID, fragment, c.seqNum); err == nil {
		c.Send += len(fragment)
	}
	c.seqNum++
}

func (c *hksvConsumer) WriteTo(io.Writer) (int64, error) {
	<-c.done
	return 0, nil
}

func (c *hksvConsumer) Stop() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.mu.Lock()
	c.active = false
	c.mu.Unlock()
	return c.Connection.Stop()
}

// acceptHDS opens a TCP listener for the HDS DataStream connection from the Home Hub
func (s *server) acceptHDS(hapConn *hap.Conn, ln net.Listener, salt string) {
	defer ln.Close()

	if tcpLn, ok := ln.(*net.TCPListener); ok {
		_ = tcpLn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	rawConn, err := ln.Accept()
	if err != nil {
		log.Error().Err(err).Str("stream", s.stream).Msg("[homekit] HKSV accept failed")
		return
	}
	defer rawConn.Close()

	// Create HDS encrypted connection (controller=false, we are accessory)
	hdsConn, err := hds.NewConn(rawConn, hapConn.SharedKey, salt, false)
	if err != nil {
		log.Error().Err(err).Str("stream", s.stream).Msg("[homekit] HKSV hds conn failed")
		return
	}

	s.AddConn(hdsConn)
	defer s.DelConn(hdsConn)

	session := newHKSVSession(s, hapConn, hdsConn)

	s.mu.Lock()
	s.hksvSession = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.hksvSession == session {
			s.hksvSession = nil
		}
		s.mu.Unlock()
		session.Close()
	}()

	log.Debug().Str("stream", s.stream).Msg("[homekit] HKSV session started")

	if err := session.Run(); err != nil {
		log.Debug().Err(err).Str("stream", s.stream).Msg("[homekit] HKSV session ended")
	}
}

// prepareHKSVConsumer pre-starts a consumer and adds it to the stream.
// When dataSend.open arrives, the consumer is ready immediately.
func (s *server) prepareHKSVConsumer() {
	stream := streams.Get(s.stream)
	if stream == nil {
		return
	}

	consumer := newHKSVConsumer()

	if err := stream.AddConsumer(consumer); err != nil {
		log.Debug().Err(err).Str("stream", s.stream).Msg("[homekit] HKSV prepare consumer failed")
		return
	}

	log.Debug().Str("stream", s.stream).Msg("[homekit] HKSV consumer prepared")

	s.mu.Lock()
	// Clean up any previous prepared consumer
	if s.preparedConsumer != nil {
		old := s.preparedConsumer
		s.preparedConsumer = nil
		s.mu.Unlock()
		stream.RemoveConsumer(old)
		_ = old.Stop()
		s.mu.Lock()
	}
	s.preparedConsumer = consumer
	s.mu.Unlock()

	// Keep alive until used or timeout (60 seconds)
	select {
	case <-consumer.done:
		// consumer was stopped (used or server closed)
	case <-time.After(60 * time.Second):
		// timeout: clean up unused prepared consumer
		s.mu.Lock()
		if s.preparedConsumer == consumer {
			s.preparedConsumer = nil
			s.mu.Unlock()
			stream.RemoveConsumer(consumer)
			_ = consumer.Stop()
			log.Debug().Str("stream", s.stream).Msg("[homekit] HKSV prepared consumer expired")
		} else {
			s.mu.Unlock()
		}
	}
}

func (s *server) takePreparedConsumer() *hksvConsumer {
	s.mu.Lock()
	defer s.mu.Unlock()
	consumer := s.preparedConsumer
	s.preparedConsumer = nil
	return consumer
}
