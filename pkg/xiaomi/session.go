package xiaomi

import (
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

var (
	sessionMu sync.Mutex
	sessions  = map[string]*session{}

	errTimeout = errors.New("miss: read raw: i/o timeout")
)

type session struct {
	client *miss.Client
	key    string

	mu         sync.Mutex
	streams    map[*stream]struct{}
	activeMask uint8
	startedMask uint8
	quality    [2]uint8

	cmdMu      sync.Mutex
	writeMu    sync.Mutex
	workerOnce sync.Once
	closeOnce  sync.Once

	speakerOnce sync.Once
	speakerErr  error

	resMap  map[videoRes]uint8
	lastSeq [2]uint32
	lastTS  [2]uint64
	seqInit [2]bool
}

type stream struct {
	session *session
	channel uint8
	ch      chan *miss.Packet

	closeOnce sync.Once
	deadline  atomic.Value
	done      chan struct{}
}

type videoRes struct {
	width  uint16
	height uint16
}

func (r videoRes) area() uint32 {
	return uint32(r.width) * uint32(r.height)
}

func getSession(rawURL string) (*session, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	key := u.Host
	if did := query.Get("did"); did != "" {
		key += "|" + did
	}

	sessionMu.Lock()
	if s := sessions[key]; s != nil {
		sessionMu.Unlock()
		return s, nil
	}
	sessionMu.Unlock()

	client, err := miss.Dial(rawURL)
	if err != nil {
		return nil, err
	}

	s := &session{
		client:  client,
		key:     key,
		streams: make(map[*stream]struct{}),
		resMap:  make(map[videoRes]uint8),
	}

	sessionMu.Lock()
	if existing := sessions[key]; existing != nil {
		sessionMu.Unlock()
		_ = client.Close()
		return existing, nil
	}
	sessions[key] = s
	sessionMu.Unlock()

	return s, nil
}

func (s *session) openStream(channel uint8) *stream {
	st := &stream{
		session: s,
		channel: channel,
		ch:      make(chan *miss.Packet, 100),
		done:    make(chan struct{}),
	}
	st.deadline.Store(time.Time{})

	s.mu.Lock()
	s.streams[st] = struct{}{}
	s.updateActiveMaskLocked()
	s.mu.Unlock()

	s.workerOnce.Do(func() {
		go s.worker()
	})

	return st
}

func (s *session) updateActiveMaskLocked() {
	var mask uint8
	for st := range s.streams {
		mask |= 1 << st.channel
	}

	if mask == s.activeMask {
		return
	}

	s.activeMask = mask
	if mask != 0b11 {
		s.resMap = make(map[videoRes]uint8)
		s.seqInit = [2]bool{}
	}
}

func (s *session) worker() {
	for {
		_ = s.client.SetDeadline(time.Now().Add(core.ConnDeadline))

		pkt, err := s.client.ReadPacket()
		if err != nil {
			s.shutdown(err)
			return
		}

		s.dispatch(pkt)
	}
}

func (s *session) dispatch(pkt *miss.Packet) {
	s.mu.Lock()
	if len(s.streams) == 0 {
		s.mu.Unlock()
		return
	}

	streams := make([]*stream, 0, len(s.streams))
	for st := range s.streams {
		streams = append(streams, st)
	}
	mask := s.activeMask
	s.mu.Unlock()

	if isAudioCodec(pkt.CodecID) {
		for _, st := range streams {
			st.push(pkt)
		}
		return
	}

	ch := s.classifyVideo(pkt, mask)
	for _, st := range streams {
		if st.channel == ch {
			st.push(pkt)
		}
	}
}

func (s *session) classifyVideo(pkt *miss.Packet, mask uint8) uint8 {
	if mask == 0b01 {
		return 0
	}
	if mask == 0b10 {
		return 1
	}
	if mask == 0 {
		return 0
	}

	res, ok := parseVideoRes(pkt)

	s.mu.Lock()
	defer s.mu.Unlock()

	var ch uint8
	if ok {
		ch = s.mapResolutionLocked(res, mask)
	} else {
		ch = s.classifyBySeqLocked(pkt, mask)
	}

	s.lastSeq[ch] = pkt.Sequence
	s.lastTS[ch] = pkt.Timestamp
	s.seqInit[ch] = true
	return ch
}

func (s *session) mapResolutionLocked(res videoRes, mask uint8) uint8 {
	if ch, ok := s.resMap[res]; ok {
		return ch
	}

	if mask == 0b01 {
		s.resMap[res] = 0
		return 0
	}
	if mask == 0b10 {
		s.resMap[res] = 1
		return 1
	}

	switch len(s.resMap) {
	case 0:
		s.resMap[res] = 0
		return 0
	case 1:
		var existing videoRes
		for r := range s.resMap {
			existing = r
			break
		}
		if res.area() > existing.area() {
			s.resMap[res] = 0
			s.resMap[existing] = 1
			return 0
		}
		s.resMap[res] = 1
		s.resMap[existing] = 0
		return 1
	default:
		var mainRes videoRes
		for r, ch := range s.resMap {
			if ch == 0 {
				mainRes = r
				break
			}
		}
		if res.area() >= mainRes.area() {
			s.resMap[res] = 0
			return 0
		}
		s.resMap[res] = 1
		return 1
	}
}

func (s *session) classifyBySeqLocked(pkt *miss.Packet, mask uint8) uint8 {
	if mask == 0b01 {
		return 0
	}
	if mask == 0b10 {
		return 1
	}

	const max = ^uint32(0)
	d0, d1 := max, max
	if s.seqInit[0] {
		d0 = pkt.Sequence - s.lastSeq[0]
	}
	if s.seqInit[1] {
		d1 = pkt.Sequence - s.lastSeq[1]
	}

	if d0 <= d1 {
		return 0
	}
	return 1
}

func (s *session) videoStart(channel, quality, audio uint8) error {
	if channel > 1 {
		return nil
	}

	s.cmdMu.Lock()
	defer s.cmdMu.Unlock()

	if s.startedMask&(1<<channel) != 0 {
		if quality != 0 {
			s.quality[channel] = quality
		}
		return nil
	}

	s.quality[channel] = quality
	other := channel ^ 1

	if s.startedMask&(1<<other) != 0 {
		if err := s.client.VideoStartDual(s.quality[0], s.quality[1], audio); err != nil {
			return err
		}
		s.startedMask |= 1 << channel
		return nil
	}

	if err := s.client.VideoStart(channel, quality, audio); err != nil {
		return err
	}
	s.startedMask |= 1 << channel
	return nil
}

func (s *session) speakerStart() error {
	s.speakerOnce.Do(func() {
		s.cmdMu.Lock()
		defer s.cmdMu.Unlock()
		s.speakerErr = s.client.SpeakerStart()
	})
	return s.speakerErr
}

func (s *session) writeAudio(codecID uint32, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.client.WriteAudio(codecID, payload)
}

func (s *session) shutdown(err error) {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		streams := make([]*stream, 0, len(s.streams))
		for st := range s.streams {
			streams = append(streams, st)
		}
		s.streams = make(map[*stream]struct{})
		s.activeMask = 0
		s.mu.Unlock()

		for _, st := range streams {
			st.close()
		}

		_ = s.client.Close()

		sessionMu.Lock()
		if sessions[s.key] == s {
			delete(sessions, s.key)
		}
		sessionMu.Unlock()

		_ = err
	})
}

func (s *session) removeStream(st *stream) {
	s.mu.Lock()
	if _, ok := s.streams[st]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.streams, st)
	s.updateActiveMaskLocked()
	empty := len(s.streams) == 0
	s.mu.Unlock()

	st.close()

	if empty {
		s.shutdown(io.EOF)
	}
}

func (s *stream) VideoStart(quality, audio uint8) error {
	return s.session.videoStart(s.channel, quality, audio)
}

func (s *stream) SpeakerStart() error {
	return s.session.speakerStart()
}

func (s *stream) WriteAudio(codecID uint32, payload []byte) error {
	return s.session.writeAudio(codecID, payload)
}

func (s *stream) RemoteAddr() net.Addr {
	return s.session.client.RemoteAddr()
}

func (s *stream) SetDeadline(t time.Time) error {
	s.deadline.Store(t)
	return nil
}

func (s *stream) wantsAudio() bool {
	return true
}

func (s *stream) ReadPacket() (*miss.Packet, error) {
	deadline, _ := s.deadline.Load().(time.Time)
	if deadline.IsZero() {
		select {
		case pkt := <-s.ch:
			return pkt, nil
		case <-s.done:
			return nil, io.EOF
		}
	}

	d := time.Until(deadline)
	if d <= 0 {
		return nil, errTimeout
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case pkt := <-s.ch:
		return pkt, nil
	case <-s.done:
		return nil, io.EOF
	case <-timer.C:
		return nil, errTimeout
	}
}

func (s *stream) Close() error {
	s.session.removeStream(s)
	return nil
}

func (s *stream) push(pkt *miss.Packet) {
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.ch <- pkt:
	default:
	}
}

func (s *stream) close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

func isAudioCodec(codecID uint32) bool {
	switch codecID {
	case miss.CodecPCM, miss.CodecPCMU, miss.CodecPCMA, miss.CodecOPUS:
		return true
	default:
		return false
	}
}

func parseVideoRes(pkt *miss.Packet) (videoRes, bool) {
	switch pkt.CodecID {
	case miss.CodecH264:
		sps := findAnnexBSPS(pkt.Payload, false)
		if sps == nil {
			return videoRes{}, false
		}
		info := h264.DecodeSPS(sps)
		if info == nil {
			return videoRes{}, false
		}
		return videoRes{width: info.Width(), height: info.Height()}, true
	case miss.CodecH265:
		sps := findAnnexBSPS(pkt.Payload, true)
		if sps == nil {
			return videoRes{}, false
		}
		info := h265.DecodeSPS(sps)
		if info == nil {
			return videoRes{}, false
		}
		return videoRes{width: info.Width(), height: info.Height()}, true
	default:
		return videoRes{}, false
	}
}

func findAnnexBSPS(payload []byte, isH265 bool) []byte {
	want := byte(7)
	if isH265 {
		want = 33
	}

	for i := 0; i+4 < len(payload); {
		start, size := findStartCode(payload, i)
		if start < 0 {
			return nil
		}

		naluStart := start + size
		next, _ := findStartCode(payload, naluStart)
		naluEnd := len(payload)
		if next > 0 {
			naluEnd = next
		}

		if naluEnd <= naluStart {
			return nil
		}

		var naluType byte
		if isH265 {
			naluType = (payload[naluStart] >> 1) & 0x3F
		} else {
			naluType = payload[naluStart] & 0x1F
		}

		if naluType == want {
			return payload[naluStart:naluEnd]
		}

		i = naluEnd
	}

	return nil
}

func findStartCode(b []byte, from int) (int, int) {
	for i := from; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		if b[i+2] == 1 {
			return i, 3
		}
		if b[i+2] == 0 && b[i+3] == 1 {
			return i, 4
		}
	}
	return -1, 0
}
