package wyoming

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/pcm/s16le"
	"github.com/pion/rtp"
)

type Server struct {
	Name string

	VADThreshold int16
	WakeURI      string

	MicHandler func(cons core.Consumer) error
	SndHandler func(prod core.Producer) error
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

func (s *Server) Handle(conn net.Conn) error {
	api := NewAPI(conn)
	sat := newSatellite(api, s)
	defer sat.Close()

	//log.Debug().Msgf("[wyoming] new client: %s", conn.RemoteAddr())

	var snd []byte

	for {
		evt, err := api.ReadEvent()
		if err != nil {
			return err
		}

		//log.Printf("%s %s %d", evt.Type, evt.Data, len(evt.Payload))

		switch evt.Type {
		case "ping": // {"text": null}
			_ = api.WriteEvent(&Event{Type: "pong", Data: evt.Data})
		case "describe":
			// {"asr": [], "tts": [], "handle": [], "intent": [], "wake": [], "satellite": {"name": "my satellite", "attribution": {"name": "", "url": ""}, "installed": true, "description": "my satellite", "version": "1.4.1", "area": null, "snd_format": null}}
			data := fmt.Sprintf(`{"satellite":{"name":%q,"attribution":{"name":"go2rtc","url":"https://github.com/AlexxIT/go2rtc"},"installed":true}}`, s.Name)
			_ = api.WriteEvent(&Event{Type: "info", Data: []byte(data)})
		case "run-satellite":
			if err = sat.run(); err != nil {
				return err
			}
		case "pause-satellite":
			sat.pause()
		case "detect": // WAKE_WORD_START {"names": null}
		case "detection": // WAKE_WORD_END {"name": "ok_nabu_v0.1", "timestamp": 17580, "speaker": null}
		case "transcribe": // STT_START {"language": "en"}
		case "voice-started": // STT_VAD_START {"timestamp": 1160}
		case "voice-stopped": // STT_VAD_END {"timestamp": 2470}
			sat.idle()
		case "transcript": // STT_END {"text": "how are you"}
		case "synthesize": // TTS_START {"text": "Sorry, I couldn't understand that", "voice": {"language": "en"}}
		case "audio-start": // TTS_END {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
			snd = snd[:0]
		case "audio-chunk": // {"rate": 22050, "width": 2, "channels": 1, "timestamp": 0}
			snd = append(snd, evt.Payload...)
		case "audio-stop": // {"timestamp": 2.880000000000002}
			sat.respond(snd)
		case "error":
			sat.start()
		}
	}
}

// states like Home Assistant
const (
	stateUnavailable = iota
	stateIdle
	stateWaitVAD // aka wait VAD
	stateWaitWakeWord
	stateStreaming
)

type satellite struct {
	api *API
	srv *Server

	state uint8
	mu    sync.Mutex

	timestamp int

	mic  *micConsumer
	wake *WakeWord
}

func newSatellite(api *API, srv *Server) *satellite {
	sat := &satellite{api: api, srv: srv}
	return sat
}

func (s *satellite) Close() error {
	s.pause()
	return s.api.Close()
}

func (s *satellite) run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != stateUnavailable {
		return errors.New("wyoming: wrong satellite state")
	}

	s.mic = newMicConsumer(s.onMicChunk)
	s.mic.RemoteAddr = s.api.conn.RemoteAddr().String()

	if err := s.srv.MicHandler(s.mic); err != nil {
		return err
	}

	s.state = stateIdle
	go s.start()

	return nil
}

func (s *satellite) pause() {
	s.mu.Lock()

	s.state = stateUnavailable
	if s.mic != nil {
		if s.mic.onClose != nil {
			s.mic.onClose()
		}
		_ = s.mic.Stop()
		s.mic = nil
	}
	if s.wake != nil {
		_ = s.wake.Close()
		s.wake = nil
	}

	s.mu.Unlock()
}

func (s *satellite) start() {
	s.mu.Lock()

	if s.state != stateUnavailable {
		s.state = stateWaitVAD
	}

	s.mu.Unlock()
}

func (s *satellite) idle() {
	s.mu.Lock()

	if s.state != stateUnavailable {
		s.state = stateIdle
	}

	s.mu.Unlock()
}

const wakeTimeout = 5 * 2 * 16000 // 5 seconds

func (s *satellite) onMicChunk(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == stateIdle {
		return
	}

	if s.state == stateWaitVAD {
		// tests show that values over 1000 are most likely speech
		if s.srv.VADThreshold == 0 || s16le.PeaksRMS(chunk) > s.srv.VADThreshold {
			if s.wake == nil && s.srv.WakeURI != "" {
				s.wake, _ = DialWakeWord(s.srv.WakeURI)
			}
			if s.wake == nil {
				// some problems with wake word - redirect to HA
				evt := &Event{
					Type: "run-pipeline",
					Data: []byte(`{"start_stage":"wake","end_stage":"tts","restart_on_end":false}`),
				}
				if err := s.api.WriteEvent(evt); err != nil {
					return
				}
				s.state = stateStreaming
			} else {
				s.state = stateWaitWakeWord
			}
			s.timestamp = 0
		}
	}

	if s.state == stateWaitWakeWord {
		if s.wake.Detection != "" {
			// check if wake word detected
			evt := &Event{
				Type: "run-pipeline",
				Data: []byte(`{"start_stage":"asr","end_stage":"tts","restart_on_end":false}`),
			}
			_ = s.api.WriteEvent(evt)
			s.state = stateStreaming
			s.timestamp = 0
		} else if err := s.wake.WriteChunk(chunk); err != nil {
			// wake word service failed
			s.state = stateWaitVAD
			_ = s.wake.Close()
			s.wake = nil
		} else if s.timestamp > wakeTimeout {
			// wake word detection timeout
			s.state = stateWaitVAD
		}
	} else if s.wake != nil {
		_ = s.wake.Close()
		s.wake = nil
	}

	if s.state == stateStreaming {
		data := fmt.Sprintf(`{"rate":16000,"width":2,"channels":1,"timestamp":%d}`, s.timestamp)
		evt := &Event{Type: "audio-chunk", Data: []byte(data), Payload: chunk}
		_ = s.api.WriteEvent(evt)
	}

	s.timestamp += len(chunk) / 2
}

func (s *satellite) respond(data []byte) {
	prod := newSndProducer(data, func() {
		_ = s.api.WriteEvent(&Event{Type: "played"})
		s.start()
	})
	if err := s.srv.SndHandler(prod); err != nil {
		prod.onClose()
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

type sndProducer struct {
	core.Connection
	data    []byte
	onClose func()
}

func newSndProducer(data []byte, onClose func()) *sndProducer {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    pcm.ProducerCodecs(),
		},
	}

	return &sndProducer{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "wyoming",
			Protocol:   "tcp",
			Medias:     medias,
		},
		data,
		onClose,
	}
}

func (s *sndProducer) Start() error {
	if len(s.Receivers) == 0 {
		return nil
	}

	var pts time.Duration
	var seq uint16

	t0 := time.Now()

	src := &core.Codec{Name: core.CodecPCML, ClockRate: 22050}
	dst := s.Receivers[0].Codec
	f := pcm.Transcode(dst, src)

	bps := uint32(pcm.BytesPerFrame(dst))

	chunkBytes := int(2 * src.ClockRate / 50) // 20ms

	for {
		n := len(s.data)
		if n == 0 {
			break
		}
		if chunkBytes > n {
			chunkBytes = n
		}

		pkt := &core.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				SequenceNumber: seq,
				Timestamp:      uint32(s.Recv/2) * bps,
			},
			Payload: f(s.data[:chunkBytes]),
		}

		if d := pts - time.Since(t0); d > 0 {
			time.Sleep(d)
		}

		s.Receivers[0].WriteRTP(pkt)

		s.Recv += chunkBytes
		s.data = s.data[chunkBytes:]

		pts += 10 * time.Millisecond
		seq++
	}

	s.onClose()

	return nil
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
