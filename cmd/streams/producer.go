package streams

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
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

	url     string
	element streamer.Producer
	tracks  []*streamer.Track

	state state
}

func (p *Producer) GetMedias() []*streamer.Media {
	if p.state == stateNone {
		log.Debug().Str("url", p.url).Msg("[streams] probe producer")

		var err error
		p.element, err = GetProducer(p.url)
		if err != nil {
			log.Error().Err(err).Str("url", p.url).Msg("[streams] probe producer")
			return nil
		}

		p.state = stateMedias
	}

	return p.element.GetMedias()
}

func (p *Producer) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
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
	if p.state != stateTracks {
		return
	}

	log.Debug().Str("url", p.url).Msg("[streams] start producer")

	p.state = stateStart
	go p.element.Start()
}

func (p *Producer) stop() {
	log.Debug().Str("url", p.url).Msg("[streams] stop producer")

	_ = p.element.Stop()
	p.element = nil
	p.tracks = nil
	p.state = stateNone
}
