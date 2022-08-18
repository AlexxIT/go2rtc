package streamer

// States, Queries and Events

type EventType byte

const (
	StateNull EventType = iota
	StateReady
	StatePaused
	StatePlaying
)

// Element base struct for all classes with support feedback
type Element struct {
	events []EventFunc
}

type EventFunc func(msg interface{})

func (e *Element) Listen(f EventFunc) {
	e.events = append(e.events, f)
}

func (e *Element) Fire(msg interface{}) {
	for _, f := range e.events {
		f(msg)
	}
}

func (e *Element) Push(msg interface{}) {
}

// Producer and Consumer interfaces

type Producer interface {
	Listen(f EventFunc)
	GetMedias() []*Media
	GetTrack(media *Media, codec *Codec) *Track
	Start() error
	Stop() error
}

type Consumer interface {
	Listen(f EventFunc)
	GetMedias() []*Media
	AddTrack(media *Media, track *Track) *Track
}
