package wyoming

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/pcm/s16le"
	"github.com/pion/rtp"
)

type Server struct {
	Name  string
	Event map[string]string

	VADThreshold int16
	WakeURI      string

	MicHandler func(cons core.Consumer) error
	SndHandler func(prod core.Producer) error

	Trace func(format string, v ...any)
	Error func(format string, v ...any)
}

func (s *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		go s.Handle(conn)
	}
}

func (s *Server) Handle(conn net.Conn) {
	api := NewAPI(conn)
	sat := newSatellite(api, s)
	defer sat.Close()

	for {
		evt, err := api.ReadEvent()
		if err != nil {
			return
		}

		switch evt.Type {
		case "ping": // {"text": null}
			_ = api.WriteEvent(&Event{Type: "pong", Data: evt.Data})
		case "audio-start": // TTS_END {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
			sat.sndAudio = sat.sndAudio[:0]
		case "audio-chunk": // {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
			sat.sndAudio = append(sat.sndAudio, evt.Payload...)
		default:
			sat.handleScript(evt)
		}
	}
}

// states like http.ConnState
const (
	stateError        = -2
	stateClosed       = -1
	stateNew          = 0
	stateIdle         = 1
	stateWaitVAD      = 2 // aka wait VAD
	stateWaitWakeWord = 3
	stateActive       = 4
)

type satellite struct {
	api *API
	srv *Server

	micState int8
	micTS    int
	micMu    sync.Mutex
	sndAudio []byte

	mic  *micConsumer
	wake *WakeWord
}

func newSatellite(api *API, srv *Server) *satellite {
	sat := &satellite{api: api, srv: srv}
	return sat
}

func (s *satellite) Close() error {
	s.Stop()
	return s.api.Close()
}

const wakeTimeout = 5 * 2 * 16000 // 5 seconds

func (s *satellite) setMicState(state int8) bool {
	s.micMu.Lock()
	defer s.micMu.Unlock()

	if s.micState == stateNew {
		s.mic = newMicConsumer(s.onMicChunk)
		s.mic.RemoteAddr = s.api.conn.RemoteAddr().String()
		if err := s.srv.MicHandler(s.mic); err != nil {
			s.micState = stateError
			s.srv.Error("can't get mic: %w", err)
			_ = s.api.Close()
		} else {
			s.micState = stateIdle
		}
	}

	if s.micState < stateIdle {
		return false
	}

	s.micState = state
	s.micTS = 0
	return true
}

func (s *satellite) micStop() {
	s.micMu.Lock()

	s.micState = stateClosed
	if s.mic != nil {
		_ = s.mic.Stop()
		s.mic = nil
	}
	if s.wake != nil {
		_ = s.wake.Close()
		s.wake = nil
	}

	s.micMu.Unlock()
}

func (s *satellite) onMicChunk(chunk []byte) {
	s.micMu.Lock()
	defer s.micMu.Unlock()

	if s.micState == stateIdle {
		return
	}

	if s.micState == stateWaitVAD {
		// tests show that values over 1000 are most likely speech
		if s.srv.VADThreshold == 0 || s16le.PeaksRMS(chunk) > s.srv.VADThreshold {
			if s.wake == nil && s.srv.WakeURI != "" {
				s.wake, _ = DialWakeWord(s.srv.WakeURI)
			}
			if s.wake == nil {
				// some problems with wake word - redirect to HA
				s.micState = stateIdle
				go s.handleScript(&Event{Type: "internal-run"})
			} else {
				s.micState = stateWaitWakeWord
			}
			s.micTS = 0
		}
	}

	if s.micState == stateWaitWakeWord {
		if s.wake.Detection != "" {
			// check if wake word detected
			s.micState = stateIdle
			go s.handleScript(&Event{Type: "internal-detection", Data: `{"name":"` + s.wake.Detection + `"}`})
		} else if err := s.wake.WriteChunk(chunk); err != nil {
			// wake word service failed
			s.micState = stateWaitVAD
			_ = s.wake.Close()
			s.wake = nil
		} else if s.micTS > wakeTimeout {
			// wake word detection timeout
			s.micState = stateWaitVAD
		}
	} else if s.wake != nil {
		_ = s.wake.Close()
		s.wake = nil
	}

	if s.micState == stateActive {
		data := fmt.Sprintf(`{"rate":16000,"width":2,"channels":1,"timestamp":%d}`, s.micTS)
		evt := &Event{Type: "audio-chunk", Data: data, Payload: chunk}
		_ = s.api.WriteEvent(evt)
	}

	s.micTS += len(chunk) / 2
}

func (s *satellite) playAudio(codec *core.Codec, rd io.Reader) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prod := pcm.OpenSync(codec, rd)
	prod.OnClose(cancel)

	if err := s.srv.SndHandler(prod); err != nil {
		return false
	} else {
		<-ctx.Done()
		return true
	}
}

type micConsumer struct {
	core.Connection
	onData  func(chunk []byte)
	onClose func()
}

func newMicConsumer(onData func(chunk []byte)) *micConsumer {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    pcm.ConsumerCodecs(),
		},
	}

	return &micConsumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "wyoming",
			Protocol:   "tcp",
			Medias:     medias,
		},
		onData: onData,
	}
}

func (c *micConsumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	src := track.Codec
	dst := &core.Codec{
		Name:      core.CodecPCML,
		ClockRate: 16000,
		Channels:  1,
	}
	sender := core.NewSender(media, dst)
	sender.Handler = pcm.TranscodeHandler(dst, src,
		repack(func(packet *core.Packet) {
			c.onData(packet.Payload)
		}),
	)
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *micConsumer) Stop() error {
	if c.onClose != nil {
		c.onClose()
	}
	return c.Connection.Stop()
}

func repack(handler core.HandlerFunc) core.HandlerFunc {
	const PacketSize = 2 * 16000 / 50 // 20ms

	var buf []byte

	return func(pkt *rtp.Packet) {
		buf = append(buf, pkt.Payload...)

		for len(buf) >= PacketSize {
			pkt = &core.Packet{Payload: buf[:PacketSize]}
			buf = buf[PacketSize:]
			handler(pkt)
		}
	}
}
