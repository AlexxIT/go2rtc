package streams

import (
	"maps"
	"net/http"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
)

// MotionHandler is called on every change of the motion state of a stream.
// It is called under the motion lock, so it should be fast and should not
// call the motion functions of this package.
type MotionHandler func(name string, motion bool)

var motions = map[string]bool{}
var motionHandlers []MotionHandler
var motionsMu sync.Mutex

// HandleMotionFunc adds a handler for motion state changes of all streams.
func HandleMotionFunc(handler MotionHandler) {
	motionsMu.Lock()
	motionHandlers = append(motionHandlers, handler)
	motionsMu.Unlock()
}

// SetMotion changes the motion state of the stream. Handlers are called
// on the first state of the stream and on every change.
func SetMotion(name string, motion bool) {
	motionsMu.Lock()
	defer motionsMu.Unlock()

	if state, ok := motions[name]; ok && state == motion {
		return
	}
	motions[name] = motion

	for _, handler := range motionHandlers {
		handler(name, motion)
	}
}

// GetMotion returns the motion state of the stream and false
// if the state was never set.
func GetMotion(name string) (motion, ok bool) {
	motionsMu.Lock()
	motion, ok = motions[name]
	motionsMu.Unlock()
	return
}

// GetMotions returns the motion states of all streams with a known state.
func GetMotions() map[string]bool {
	motionsMu.Lock()
	defer motionsMu.Unlock()
	return maps.Clone(motions)
}

// apiMotion - motion state of streams from any external detector
//
//	GET    /api/motion - motion states of all streams with a known state
//	POST   /api/motion?src=camera1 - motion started
//	DELETE /api/motion?src=camera1 - motion ended
func apiMotion(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		api.ResponseJSON(w, GetMotions())
		return
	}

	src := r.URL.Query().Get("src")
	if Get(src) == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	var motion bool
	switch r.Method {
	case "POST":
		motion = true
	case "DELETE":
	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	log.Debug().Msgf("[streams] motion src=%s motion=%t", src, motion)

	SetMotion(src, motion)
}
