package homekit

import (
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
)

// Event is one entry in the Camera Event Queue
type Event struct {
	Sequence uint64
	Type     byte
	Session  uint64
	Motion   bool
	CMAFErr  byte
	Time     time.Time
}

// EventQueue is a bounded, sequence-numbered camera event queue
type EventQueue struct {
	mu     sync.Mutex
	seq    uint64
	events []Event
	max    int
}

// NewEventQueue creates a queue retaining at most max events
func NewEventQueue(max int) *EventQueue {
	if max <= 0 {
		max = 256
	}
	return &EventQueue{max: max}
}

// Push appends an event and returns its sequence number
func (q *EventQueue) Push(ev Event) uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	ev.Sequence = q.seq
	ev.Time = time.Now()
	q.events = append(q.events, ev)
	if len(q.events) > q.max {
		q.events = append([]Event(nil), q.events[len(q.events)-q.max:]...)
	}
	return ev.Sequence
}

// Sequence is the latest sequence number
func (q *EventQueue) Sequence() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.seq
}

// Query returns up to limit events with sequence > after
func (q *EventQueue) Query(after, limit uint64) []Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit == 0 {
		limit = 32
	}
	var out []Event
	for _, ev := range q.events {
		if ev.Sequence <= after {
			continue
		}
		out = append(out, ev)
		if uint64(len(out)) >= limit {
			break
		}
	}
	return out
}

// Acknowledge drops events with sequence <= seq
func (q *EventQueue) Acknowledge(seq uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := 0
	for i < len(q.events) && q.events[i].Sequence <= seq {
		i++
	}
	if i > 0 {
		q.events = append([]Event(nil), q.events[i:]...)
	}
}

// UploadSession tracks an in-flight buffer upload
type UploadSession struct {
	ID          uint64
	ClipID      uint64
	Command     byte
	Start       time.Time
	Stop        time.Time
	StopAct     byte
	Active      bool
	Cancel      chan struct{}
	stopEmitted bool
}

// RecordingManager owns pre-buffer, CMAF publish, credentials and events
type RecordingManager struct {
	mu sync.Mutex

	Creds  *Credentials
	Buffer *RingBuffer
	Events *EventQueue

	Consumer *BufferConsumer

	recordingActive bool
	audioActive     bool

	sessions map[uint64]*UploadSession
	nextClip uint64

	// OnEventSeq is called when the event sequence changes (HAP notify)
	OnEventSeq func(seq uint32)
}

// NewRecordingManager constructs a full HKSV recording stack
func NewRecordingManager() *RecordingManager {
	return &RecordingManager{
		Creds:    NewCredentials(),
		Buffer:   NewRingBuffer(8 * time.Second),
		Events:   NewEventQueue(256),
		sessions: make(map[uint64]*UploadSession),
		nextClip: 1,
	}
}

// SetRecordingActive enables/disables event recording intent
func (m *RecordingManager) SetRecordingActive(v bool) {
	m.mu.Lock()
	m.recordingActive = v
	m.mu.Unlock()
}

// SetAudioActive enables/disables audio in recordings
func (m *RecordingManager) SetAudioActive(v bool) {
	m.mu.Lock()
	m.audioActive = v
	m.mu.Unlock()
}

// RecordingActive reports Active characteristic state
func (m *RecordingManager) RecordingActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recordingActive
}

// AudioActive reports Recording Audio Active state
func (m *RecordingManager) AudioActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.audioActive
}

// ReportMotion pushes a motion event onto the queue
func (m *RecordingManager) ReportMotion(active bool) {
	seq := m.Events.Push(Event{
		Type:   camera.BufferEventTypeMotion,
		Motion: active,
	})
	m.fireSeq(seq)
}

// HandleActivity records a buffer activity window
func (m *RecordingManager) HandleActivity(req *camera.BufferActivityCommandRequest) {
	if req.Activity == camera.BufferActivityShouldRecord {
		m.ReportMotion(true)
	} else if req.Activity == camera.BufferActivityShouldNotRecord {
		m.ReportMotion(false)
	}
}

// HandleUpload processes Buffer Upload Command and returns Clip ID
func (m *RecordingManager) HandleUpload(req *camera.BufferUploadCommandRequest) *camera.BufferUploadCommandResponse {
	// Stop must not replace an in-flight session (would drop its Cancel channel)
	if req.Command == camera.BufferUploadStop {
		clipID := m.stopUpload(req.SessionID, req.StopAction)
		return &camera.BufferUploadCommandResponse{ClipID: clipID}
	}

	m.mu.Lock()
	clipID := m.nextClip
	m.nextClip++
	// cancel any previous session with the same id before replacing
	if prev, ok := m.sessions[req.SessionID]; ok && prev.Active {
		select {
		case <-prev.Cancel:
		default:
			close(prev.Cancel)
		}
		prev.Active = false
	}
	sess := &UploadSession{
		ID:      req.SessionID,
		ClipID:  clipID,
		Command: req.Command,
		Start:   NTPToTime(req.Start),
		Stop:    NTPToTime(req.Stop),
		StopAct: req.StopAction,
		Active:  true,
		Cancel:  make(chan struct{}),
	}
	m.sessions[req.SessionID] = sess
	m.mu.Unlock()

	switch req.Command {
	case camera.BufferUploadStart, camera.BufferUploadStartAndStop:
		go m.runUpload(sess)
	}

	return &camera.BufferUploadCommandResponse{ClipID: clipID}
}

// stopUpload cancels an in-flight upload. Returns the clip id (0 if unknown)
func (m *RecordingManager) stopUpload(sessionID uint64, action byte) uint64 {
	m.mu.Lock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return 0
	}
	sess.StopAct = action
	wasActive := sess.Active
	if wasActive {
		select {
		case <-sess.Cancel:
		default:
			close(sess.Cancel)
		}
	}
	// Publish already finished for a Start session: emit stop on finalize here
	needStop := !wasActive && action == camera.BufferStopActionFinalize && !sess.stopEmitted
	if needStop {
		sess.stopEmitted = true
	}
	clipID := sess.ClipID
	m.mu.Unlock()

	if needStop {
		seq := m.Events.Push(Event{
			Type:    camera.BufferEventTypeCMAFSessionStop,
			Session: sessionID,
		})
		m.fireSeq(seq)
	}
	return clipID
}

func (m *RecordingManager) runUpload(sess *UploadSession) {
	defer func() {
		m.mu.Lock()
		if s, ok := m.sessions[sess.ID]; ok {
			s.Active = false
		}
		m.mu.Unlock()
	}()

	if m.canceled(sess) {
		return
	}

	seq := m.Events.Push(Event{
		Type:    camera.BufferEventTypeCMAFSessionStart,
		Session: sess.ID,
	})
	m.fireSeq(seq)

	if m.canceled(sess) {
		m.emitSessionStop(sess)
		return
	}

	err := m.publishSession(sess)
	if m.canceled(sess) && err == nil {
		err = &cmafPublishError{code: CMAFErrCanceled, err: errString("homekit: upload canceled")}
	}
	if err != nil {
		code := mapPublishError(err)
		seq = m.Events.Push(Event{
			Type:    camera.BufferEventTypeCMAFError,
			Session: sess.ID,
			CMAFErr: byte(code),
		})
		m.fireSeq(seq)
	}

	m.mu.Lock()
	stopAct := sess.StopAct
	cmd := sess.Command
	m.mu.Unlock()

	// StartAndStop always finalizes; Start finalizes only after Stop(finalize)
	if cmd == camera.BufferUploadStartAndStop || stopAct == camera.BufferStopActionFinalize {
		m.emitSessionStop(sess)
	}
}

func (m *RecordingManager) emitSessionStop(sess *UploadSession) {
	m.mu.Lock()
	if sess.stopEmitted {
		m.mu.Unlock()
		return
	}
	sess.stopEmitted = true
	m.mu.Unlock()

	seq := m.Events.Push(Event{
		Type:    camera.BufferEventTypeCMAFSessionStop,
		Session: sess.ID,
	})
	m.fireSeq(seq)
}

func (m *RecordingManager) canceled(sess *UploadSession) bool {
	select {
	case <-sess.Cancel:
		return true
	default:
		return false
	}
}

func (m *RecordingManager) publishSession(sess *UploadSession) error {
	start := sess.Start
	stop := sess.Stop
	if stop.IsZero() {
		stop = time.Now()
	}
	if start.IsZero() {
		start = m.Buffer.OldestWall()
		if start.IsZero() {
			start = time.Now().Add(-4 * time.Second)
		}
	}

	packets := m.Buffer.Slice(start, stop)

	m.mu.Lock()
	audioOn := m.audioActive
	m.mu.Unlock()

	if !audioOn {
		filtered := make([]Packet, 0, len(packets))
		for _, p := range packets {
			if p.Track == 0 {
				filtered = append(filtered, p)
			}
		}
		packets = filtered
	}

	// Content key is retained for future CENC; upload clear CMAF so the
	// publishing point receives a standards-compliant track (mTLS protects transport)
	clip, err := BuildClip(packets, nil)
	if err != nil {
		return &cmafPublishError{code: CMAFErrMP4Error, err: err}
	}

	url := m.Creds.PublishingPoint()
	if url == "" {
		return &cmafPublishError{code: CMAFErrInvalidState, err: errString("homekit: publishing point not set")}
	}

	cfg, tlsErr := m.Creds.TLSConfig()
	// plain HTTP is allowed without client cert (local ingest tests)
	isHTTP := len(url) >= 7 && url[:7] == "http://"
	if tlsErr != nil && !isHTTP {
		return &cmafPublishError{code: CMAFErrCertConnectionFailure, err: tlsErr}
	}
	if isHTTP {
		cfg = nil
	}

	client := NewCMAFClient(url, cfg)
	code, err := client.PublishClip(sess.ID, clip)
	if err != nil {
		return &cmafPublishError{code: code, err: err}
	}
	return nil
}

type cmafPublishError struct {
	code int
	err  error
}

func (e *cmafPublishError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return "cmaf publish error"
}

func mapPublishError(err error) int {
	if pe, ok := err.(*cmafPublishError); ok {
		return pe.code
	}
	if err == nil {
		return CMAFErrNone
	}
	return mapHTTPError(err)
}

// HandleEventCommand processes Buffer Event Command (query / acknowledge)
func (m *RecordingManager) HandleEventCommand(req *camera.BufferEventCommandRequest) *camera.BufferEventCommandResponse {
	res := &camera.BufferEventCommandResponse{}
	switch req.Command {
	case camera.BufferEventQuery:
		evs := m.Events.Query(req.SequenceNumber, req.Limit)
		for _, ev := range evs {
			item := camera.CameraBufferEvent{
				SequenceNumber: ev.Sequence,
				Type:           ev.Type,
			}
			switch ev.Type {
			case camera.BufferEventTypeCMAFSessionStart:
				item.CMAFSessionStart = camera.CameraBufferEventCMAFSession{CMAFSessionID: ev.Session}
			case camera.BufferEventTypeCMAFSessionStop:
				item.CMAFSessionStop = camera.CameraBufferEventCMAFSession{CMAFSessionID: ev.Session}
			case camera.BufferEventTypeMotion:
				item.Motion = camera.CameraBufferEventMotion{Active: ev.Motion}
			case camera.BufferEventTypeCMAFError:
				item.CMAFError = camera.CameraBufferEventCMAFError{
					CMAFSessionID: ev.Session,
					CMAFError:     ev.CMAFErr,
				}
			}
			res.Events = append(res.Events, item)
		}
	case camera.BufferEventAcknowledge:
		m.Events.Acknowledge(req.SequenceNumber)
	}
	return res
}

// EventSequence returns the current event sequence as uint32 for the characteristic
func (m *RecordingManager) EventSequence() uint32 {
	return uint32(m.Events.Sequence())
}

func (m *RecordingManager) fireSeq(seq uint64) {
	if m.OnEventSeq != nil {
		m.OnEventSeq(uint32(seq))
	}
}

// EnsureConsumer returns a buffer consumer, creating one if needed
func (m *RecordingManager) EnsureConsumer() *BufferConsumer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Consumer == nil {
		m.Consumer = NewBufferConsumer(m.Buffer)
	}
	return m.Consumer
}
