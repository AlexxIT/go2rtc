// Package streamer
//
// 1. Consumer.GetMedias - return list of Media, that Consumer can play/load/consume:
//   - Media with DirectionRecvonly for audio/video
//   - Media with DirectionSendonly for backchannel
//
// 2. Producer.GetMedias - return list of Media, that Producer can generate/create/produce
//   - Media with DirectionSendonly for audio/video
//   - Media with DirectionRecvonly for backchannel
//
// 3. Producer.GetTrack - get Media from Producer and Codec from that Media return Track from Producer:
//   - Media with DirectionSendonly should Track.WriteRTP after Producer.Start
//   - Media with DirectionRecvonly should Track.Bind and wait Track.WriteRTP from Consumer
//
// 4. Consumer.AddTrack - takes Media from Consumer and Track from Producer:
//   - Media with DirectionRecvonly should Track.WriteRTP
//   - Media with DirectionSendonly should Track.Bind
//
// 5. Producer.Start - run loop with reading rtp.Packet from source
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
