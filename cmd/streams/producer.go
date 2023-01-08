package streams

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
	"sync"
	"time"
)

type state byte

const (
	stateNone state = iota
	stateMedias
	stateTracks
	stateStart
	stateExternal
)

type Producer struct {
	streamer.Element

	url      string
	template string

	element streamer.Producer
	lastErr error
	tracks  []*streamer.Track

	state   state
	mu      sync.Mutex
	restart *time.Timer
}

func (p *Producer) SetSource(s string) {
	if p.template == "" {
		p.template = p.url
	}
	p.url = strings.Replace(p.template, "{input}", s, 1)
}

func (p *Producer) GetMedias() []*streamer.Media {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		log.Debug().Msgf("[streams] probe producer url=%s", p.url)

		p.element, p.lastErr = GetProducer(p.url)
		if p.lastErr != nil || p.element == nil {
			log.Error().Err(p.lastErr).Str("url", p.url).Caller().Send()
			return nil
		}

		p.state = stateMedias
	}

	// if element in reconnect state
	if p.element == nil {
		return nil
	}

	return p.element.GetMedias()
}

func (p *Producer) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return nil
	}

	for _, track := range p.tracks {
		if track.Codec == codec {
			return track
		}
	}

	track := p.element.GetTrack(media, codec)
	if track == nil {
		return nil
	}

	p.tracks = append(p.tracks, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return track
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
	go func() {
		// safe read element while mu locked
		if err := p.element.Start(); err != nil {
			log.Warn().Err(err).Str("url", p.url).Caller().Send()
		}
		p.reconnect()
	}()
}

func (p *Producer) reconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != stateStart {
		log.Trace().Msgf("[streams] stop reconnect url=%s", p.url)
		return
	}

	log.Debug().Msgf("[streams] reconnect to url=%s", p.url)

	p.element, p.lastErr = GetProducer(p.url)
	if p.lastErr != nil || p.element == nil {
		log.Debug().Err(p.lastErr).Caller().Send()
		// TODO: dynamic timeout
		p.restart = time.AfterFunc(30*time.Second, p.reconnect)
		return
	}

	medias := p.element.GetMedias()

	// convert all old producer tracks to new tracks
	for i, oldTrack := range p.tracks {
		// match new element medias with old track codec
		for _, media := range medias {
			codec := media.MatchCodec(oldTrack.Codec)
			if codec == nil {
				continue
			}

			// move sink from old track to new track
			newTrack := p.element.GetTrack(media, codec)
			newTrack.GetSink(oldTrack)
			p.tracks[i] = newTrack

			break
		}
	}

	go func() {
		if err := p.element.Start(); err != nil {
			log.Debug().Err(err).Caller().Send()
		}
		p.reconnect()
	}()
}

func (p *Producer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case stateExternal:
		log.Debug().Msgf("[streams] can't stop external producer")
		return
	case stateNone:
		log.Debug().Msgf("[streams] can't stop none producer")
		return
	}

	log.Debug().Msgf("[streams] stop producer url=%s", p.url)

	if p.element != nil {
		_ = p.element.Stop()
		p.element = nil
	}
	if p.restart != nil {
		p.restart.Stop()
		p.restart = nil
	}

	p.state = stateNone
	p.tracks = nil
}
