// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/rs/zerolog"
)

// hksvSession manages the HDS DataStream connection for HKSV recording
type hksvSession struct {
	server  *Server
	hapConn *hap.Conn
	hdsConn *hds.Conn
	session *hds.Session
	log     zerolog.Logger

	mu       sync.Mutex
	consumer *HKSVConsumer
}

func newHKSVSession(srv *Server, hapConn *hap.Conn, hdsConn *hds.Conn) *hksvSession {
	session := hds.NewSession(hdsConn)
	hs := &hksvSession{
		server:  srv,
		hapConn: hapConn,
		hdsConn: hdsConn,
		session: session,
		log:     srv.log,
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

	hs.log.Debug().Str("stream", hs.server.stream).Int("streamID", streamID).Msg("[hksv] dataSend open")

	if hs.consumer != nil {
		hs.stopRecording()
	}

	// Try to use the pre-started consumer from pair-verify
	consumer := hs.server.takePreparedConsumer()
	if consumer != nil {
		hs.log.Debug().Str("stream", hs.server.stream).Msg("[hksv] using prepared consumer")
		hs.consumer = consumer
		hs.server.AddConn(consumer)

		// Activate: set the HDS session and send init + start streaming
		if err := consumer.Activate(hs.session, streamID); err != nil {
			hs.log.Error().Err(err).Str("stream", hs.server.stream).Msg("[hksv] activate failed")
			hs.stopRecording()
			return nil
		}
		return nil
	}

	// Fallback: create new consumer (will be slow ~3s)
	hs.log.Debug().Str("stream", hs.server.stream).Msg("[hksv] no prepared consumer, creating new")
	consumer = NewHKSVConsumer(hs.log)

	if err := hs.server.streams.AddConsumer(hs.server.stream, consumer); err != nil {
		hs.log.Error().Err(err).Str("stream", hs.server.stream).Msg("[hksv] add consumer failed")
		return nil
	}

	hs.consumer = consumer
	hs.server.AddConn(consumer)

	go func() {
		if err := consumer.Activate(hs.session, streamID); err != nil {
			hs.log.Error().Err(err).Str("stream", hs.server.stream).Msg("[hksv] activate failed")
		}
	}()

	return nil
}

func (hs *hksvSession) handleClose(streamID int) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.log.Debug().Str("stream", hs.server.stream).Int("streamID", streamID).Msg("[hksv] dataSend close")

	if hs.consumer != nil {
		hs.stopRecording()
	}
	return nil
}

func (hs *hksvSession) stopRecording() {
	consumer := hs.consumer
	hs.consumer = nil

	hs.server.streams.RemoveConsumer(hs.server.stream, consumer)
	_ = consumer.Stop()
	hs.server.DelConn(consumer)
}
