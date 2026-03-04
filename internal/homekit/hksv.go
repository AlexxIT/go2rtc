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

	consumer := newHKSVConsumer(hs.session, streamID)
	hs.consumer = consumer

	stream := streams.Get(hs.server.stream)
	if err := stream.AddConsumer(consumer); err != nil {
		log.Error().Err(err).Str("stream", hs.server.stream).Msg("[homekit] HKSV add consumer failed")
		hs.consumer = nil
		return nil // don't kill the session
	}

	hs.server.AddConn(consumer)

	// wait for tracks to be added, then send init
	go func() {
		if err := consumer.waitAndSendInit(); err != nil {
			log.Error().Err(err).Str("stream", hs.server.stream).Msg("[homekit] HKSV send init failed")
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

// hksvConsumer implements core.Consumer, generates fMP4 and sends over HDS
type hksvConsumer struct {
	core.Connection
	session  *hds.Session
	muxer    *mp4.Muxer
	streamID int
	seqNum   int
	mu       sync.Mutex
	start    bool
	done     chan struct{}
}

func newHKSVConsumer(session *hds.Session, streamID int) *hksvConsumer {
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
		session:  session,
		muxer:    &mp4.Muxer{},
		streamID: streamID,
		done:     make(chan struct{}),
	}
}

func (c *hksvConsumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := byte(len(c.Senders))

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				if !h264.IsKeyframe(packet.Payload) {
					return
				}
				c.start = true
			}

			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if err := c.session.SendMediaFragment(c.streamID, b, c.seqNum); err == nil {
				c.Send += len(b)
				c.seqNum++
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecAAC:
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				return
			}

			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if err := c.session.SendMediaFragment(c.streamID, b, c.seqNum); err == nil {
				c.Send += len(b)
				c.seqNum++
			}
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

	return nil
}

func (c *hksvConsumer) waitAndSendInit() error {
	// wait for at least one track to be added
	for i := 0; i < 50; i++ {
		if len(c.Senders) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	init, err := c.muxer.GetInit()
	if err != nil {
		return err
	}
	return c.session.SendMediaInit(c.streamID, init)
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
