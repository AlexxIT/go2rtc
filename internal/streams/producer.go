package streams

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type state byte

const (
	stateNone state = iota
	stateMedias
	stateTracks
	stateStart
	stateExternal
	stateInternal
)

type Producer struct {
	core.Listener

	url      string
	template string

	conn      core.Producer
	receivers []*core.Receiver
	senders   []*core.Receiver

	// Mixers for backchannel - one per codec
	mixers []*core.RTPMixer

	state         state
	mu            sync.Mutex
	workerID      int
	mixingEnabled bool // Whether to enable audio mixing for multiple backchannel consumers (default: false)
}

const SourceTemplate = "{input}"

func NewProducer(source string) *Producer {
	// Parse #mix flag
	mixingEnabled := strings.Contains(source, "#mix")
	if mixingEnabled {
		source = strings.ReplaceAll(source, "#mix", "")
	}

	if strings.Contains(source, SourceTemplate) {
		return &Producer{
			template:      source,
			mixingEnabled: mixingEnabled,
		}
	}

	return &Producer{
		url:           source,
		mixingEnabled: mixingEnabled,
	}
}

func (p *Producer) SetSource(s string) {
	// Parse #mix flag
	p.mixingEnabled = strings.Contains(s, "#mix")
	if p.mixingEnabled {
		s = strings.ReplaceAll(s, "#mix", "")
	}

	if p.template == "" {
		p.url = s
	} else {
		p.url = strings.Replace(p.template, SourceTemplate, s, 1)
	}
}

func (p *Producer) Dial() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		conn, err := GetProducer(p.url)
		if err != nil {
			return err
		}

		p.conn = conn
		p.state = stateMedias
	}

	return nil
}

func (p *Producer) GetMedias() []*core.Media {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return nil
	}

	return p.conn.GetMedias()
}

func (p *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return nil, errors.New("get track from none state")
	}

	for _, track := range p.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track, err := p.conn.GetTrack(media, codec)
	if err != nil {
		return nil, err
	}

	p.receivers = append(p.receivers, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return track, nil
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return errors.New("add track from none state")
	}

	// If mixing is enabled, use mixer for multiple backchannel consumers
	if p.mixingEnabled {
		// Check if we already have a mixer for this codec
		for _, mixer := range p.mixers {
			if mixer.Codec.Match(codec) {
				// Register this consumer receiver as a parent
				mixer.AddParent(&track.Node)
				p.senders = append(p.senders, track)
				return nil
			}
		}

		// No mixer exists yet, create one
		mixer := core.NewRTPMixer(ffmpegBin, media, codec)
		mixer.AddParent(&track.Node)

		// Connect mixer to underlying protocol
		consumer := p.conn.(core.Consumer)
		mixerReceiver := core.NewReceiver(media, codec)
		mixerReceiver.ParentNode = &mixer.Node
		if err := consumer.AddTrack(media, codec, mixerReceiver); err != nil {
			return err
		}

		p.mixers = append(p.mixers, mixer)
		p.senders = append(p.senders, track)
	} else {
		// Without mixing, directly pass track to underlying protocol
		if err := p.conn.(core.Consumer).AddTrack(media, codec, track); err != nil {
			return err
		}

		p.senders = append(p.senders, track)
	}

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return nil
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn := p.conn; conn != nil {
		connData, err := json.Marshal(conn)
		if err != nil {
			return nil, err
		}

		// If no mixers, return as-is
		if len(p.mixers) == 0 {
			return connData, nil
		}

		// Marshal mixers
		mixersData, err := json.Marshal(p.mixers)
		if err != nil {
			return nil, err
		}

		// Simply append mixers field at the end
		// Remove closing } and add mixers field
		result := connData[:len(connData)-1] // Remove }
		result = append(result, []byte(`,"mixers":`)...)
		result = append(result, mixersData...)
		result = append(result, '}')

		return result, nil
	}

	info := map[string]string{"url": p.url}
	return json.Marshal(info)
}

// internals

func (p *Producer) start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != stateTracks {
		return
	}

	log.Debug().Msgf("[streams] start producer url=%s", p.url)

	p.state = stateStart
	p.workerID++

	go p.worker(p.conn, p.workerID)
}

func (p *Producer) worker(conn core.Producer, workerID int) {
	if err := conn.Start(); err != nil {
		p.mu.Lock()
		closed := p.workerID != workerID
		p.mu.Unlock()

		if closed {
			return
		}

		log.Warn().Err(err).Str("url", p.url).Caller().Send()
	}

	p.reconnect(workerID, 0)
}

func (p *Producer) reconnect(workerID, retry int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.workerID != workerID {
		log.Trace().Msgf("[streams] stop reconnect url=%s", p.url)
		return
	}

	log.Debug().Msgf("[streams] retry=%d to url=%s", retry, p.url)

	conn, err := GetProducer(p.url)
	if err != nil {
		log.Debug().Msgf("[streams] producer=%s", err)

		timeout := time.Minute
		if retry < 5 {
			timeout = time.Second
		} else if retry < 10 {
			timeout = time.Second * 5
		} else if retry < 20 {
			timeout = time.Second * 10
		}

		time.AfterFunc(timeout, func() {
			p.reconnect(workerID, retry+1)
		})
		return
	}

	for _, media := range conn.GetMedias() {
		switch media.Direction {
		case core.DirectionRecvonly:
			for i, receiver := range p.receivers {
				codec := media.MatchCodec(receiver.Codec)
				if codec == nil {
					continue
				}

				track, err := conn.GetTrack(media, codec)
				if err != nil {
					continue
				}

				receiver.Replace(track)
				p.receivers[i] = track
				break
			}

		case core.DirectionSendonly:
			for _, sender := range p.senders {
				codec := media.MatchCodec(sender.Codec)
				if codec == nil {
					continue
				}

				_ = conn.(core.Consumer).AddTrack(media, codec, sender)
			}
		}
	}

	// stop previous connection after moving tracks (fix ghost exec/ffmpeg)
	_ = p.conn.Stop()
	// swap connections
	p.conn = conn

	go p.worker(conn, workerID)
}

func (p *Producer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case stateExternal:
		log.Trace().Msgf("[streams] skip stop external producer")
		return
	case stateNone:
		log.Trace().Msgf("[streams] skip stop none producer")
		return
	case stateStart:
		p.workerID++
	}

	log.Debug().Msgf("[streams] stop producer url=%s", p.url)

	if p.conn != nil {
		_ = p.conn.Stop()
		p.conn = nil
	}

	p.state = stateNone
	p.receivers = nil
	p.senders = nil
}
