package streams

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
	"sync"
)

type state byte

const (
	stateNone state = iota
	stateMedias
	stateTracks
	stateStart
)

type Producer struct {
	streamer.Element

	url      string
	template string

	element streamer.Producer
	tracks  []*streamer.Track

	state state
	mx    sync.Mutex
}

func (p *Producer) SetSource(s string) {
	if p.template == "" {
		p.template = p.url
	}
	p.url = strings.Replace(p.template, "{input}", s, 1)
}

func (p *Producer) GetMedias() []*streamer.Media {
	p.mx.Lock()
	defer p.mx.Unlock()

	if p.state == stateNone {
		log.Debug().Str("url", p.url).Msg("[streams] probe producer")

		var err error
		p.element, err = GetProducer(p.url)
		if err != nil || p.element == nil {
			log.Error().Err(err).Str("url", p.url).Msg("[streams] probe producer")
			return nil
		}

		p.state = stateMedias
	}

	return p.element.GetMedias()
}

func (p *Producer) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	p.mx.Lock()
	defer p.mx.Unlock()

	if p.state == stateMedias {
		p.state = stateTracks
	}

	track := p.element.GetTrack(media, codec)

	for _, t := range p.tracks {
		if track == t {
			return track
		}
	}

	p.tracks = append(p.tracks, track)

	return track
}

// internals

func (p *Producer) start() {
	p.mx.Lock()
	defer p.mx.Unlock()

	if p.state != stateTracks {
		return
	}

	log.Debug().Str("url", p.url).Msg("[streams] start producer")

	p.state = stateStart
	go func() {
		if err := p.element.Start(); err != nil {
			log.Warn().Err(err).Str("url", p.url).Msg("[streams] start")
		}
	}()
}

func (p *Producer) stop() {
	p.mx.Lock()

	log.Debug().Str("url", p.url).Msg("[streams] stop producer")

	if p.element != nil {
		_ = p.element.Stop()
		p.element = nil
	} else {
		log.Warn().Str("url", p.url).Msg("[streams] stop empty producer")
	}
	p.tracks = nil
	p.state = stateNone

	p.mx.Unlock()
}
