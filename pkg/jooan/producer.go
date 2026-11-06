package jooan

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

var (
	ErrNotStarted = errors.New("jooan: talkback is not started")
	ErrNoTwoWay   = errors.New("jooan: camera does not advertise two-way streaming")
)

// Producer is a send-only PCMU talkback source.
type Producer struct {
	core.Connection
	session Session
	config  Config

	mu        sync.Mutex
	started   bool
	stopped   bool
	frameBuf  []byte
	timestamp uint32
	sendErr   error
	done      chan struct{}
	doneOnce  sync.Once
	attached  bool
}

func NewProducer(config Config) (*Producer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	session, err := newSession(config)
	if err != nil {
		return nil, fmt.Errorf("jooan: connect: %w", err)
	}
	timeout := time.Duration(config.LoginTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if err = session.AVClientStart(timeout); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("jooan: av login: %w", err)
	}
	if !session.HasTwoWayStreaming() {
		_ = session.Close()
		return nil, ErrNoTwoWay
	}

	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{{
			Name:      core.CodecPCMU,
			ClockRate: AudioSampleRate,
			Channels:  AudioChannels,
		}},
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "jooan",
			Protocol:   "tutk+dtls",
			RemoteAddr: config.String(),
			Medias:     []*core.Media{media},
			Transport:  session,
		},
		session: session,
		config:  config,
		done:    make(chan struct{}),
	}, nil
}

func newProducerWithSession(config Config, session Session) (*Producer, error) {
	if session == nil {
		return nil, errors.New("jooan: nil session")
	}
	if !session.HasTwoWayStreaming() {
		return nil, ErrNoTwoWay
	}
	media := &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{{Name: core.CodecPCMU, ClockRate: AudioSampleRate, Channels: AudioChannels}},
	}
	return &Producer{
		Connection: core.Connection{ID: core.NewID(), FormatName: "jooan", Protocol: "tutk+dtls",
			RemoteAddr: config.String(), Medias: []*core.Media{media}, Transport: session},
		session: session, config: config, done: make(chan struct{}),
	}, nil
}

func (p *Producer) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if media == nil || codec == nil || track == nil || codec.Name != core.CodecPCMU ||
		media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly ||
		codec.ClockRate != AudioSampleRate || codec.Channels != AudioChannels {
		return errors.New("jooan: only PCMU/16000/mono audio is supported")
	}

	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return errors.New("jooan: producer is stopped")
	}
	if p.attached {
		p.mu.Unlock()
		return errors.New("jooan: audio sender already attached")
	}
	if !p.started {
		if err := p.session.AVTwoWayStart(p.config.speakerChannel()); err != nil {
			p.mu.Unlock()
			return fmt.Errorf("jooan: speaker start: %w", err)
		}
		p.started = true
	}

	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		p.write(packet.Payload)
	}
	sender.HandleRTP(track)
	p.attached = true
	p.Senders = append(p.Senders, sender)
	p.mu.Unlock()
	return nil
}

func (p *Producer) write(payload []byte) {
	p.mu.Lock()
	if p.stopped || !p.started || p.sendErr != nil {
		p.mu.Unlock()
		return
	}
	p.frameBuf = append(p.frameBuf, payload...)
	for len(p.frameBuf) >= AudioFrameBytes {
		frame := append([]byte(nil), p.frameBuf[:AudioFrameBytes]...)
		p.frameBuf = p.frameBuf[AudioFrameBytes:]
		if err := p.session.AVSendAudioDataTwoWay(tutk.CodecPCMU, frame, p.timestamp, AudioSampleRate, AudioChannels); err != nil {
			p.sendErr = fmt.Errorf("jooan: send audio: %w", err)
			p.doneOnce.Do(func() { close(p.done) })
			p.mu.Unlock()
			_ = p.Stop()
			return
		}
		p.Send += len(frame)
		p.timestamp += AudioFrameTimestamp
	}
	p.mu.Unlock()
}

func (p *Producer) Start() error {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sendErr
}

func (p *Producer) Stop() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	started := p.started
	p.started = false
	senders := append([]*core.Sender(nil), p.Senders...)
	p.mu.Unlock()

	for _, sender := range senders {
		sender.Close()
	}

	var err error
	if started {
		err = p.session.AVTwoWayStop(p.config.speakerChannel())
	}
	p.doneOnce.Do(func() { close(p.done) })
	closeErr := p.session.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
