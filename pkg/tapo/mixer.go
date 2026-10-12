package tapo

import (
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const (
	// mixerChunkSize is the size of one chunk sent to the camera: 40 ms of
	// PCMA/8000, the same as backchannelFramesPerPart RTP packets of 20 ms.
	mixerChunkSize = 160 * backchannelFramesPerPart
	mixerChunkTime = backchannelFramesPerPart * 20 * time.Millisecond

	// mixerMaxQueue limits the queue of one source (8000 bytes = 1 s).
	mixerMaxQueue = 60 * 8000

	// mixerIdleEnd is the pause after which the audio is considered finished.
	// The next audio starts with a new pre-fill and lead-in.
	mixerIdleEnd = 500 * time.Millisecond

	// mixerForget is the pause after which an empty source is dropped.
	mixerForget = 5 * time.Second

	// mixerKnee is the level where the limiter of the mixed signal starts.
	mixerKnee = 24000
)

// pcmaSilence is the G.711 A-law code of zero amplitude.
const pcmaSilence = 0xD5

// mixerOptions are the settings of the paced backchannel, see README.
type mixerOptions struct {
	prefillChunks int // silence sent at once before the audio
	leadChunks    int // silence sent in real time before the audio
	startBytes    int // audio collected before the first chunk is sent
}

// mixerStats describes one continuous piece of audio sent to the camera.
type mixerStats struct {
	Chunks    int // sent chunks with audio
	Mixed     int // chunks with more than one source
	Underruns int // ticks without audio in the middle of the audio
	Late      int // chunks sent more than half a chunk too late
	MaxQueue  int // longest queue of a source, bytes
	MaxGap    time.Duration
	MaxWrite  time.Duration
}

// mixerSource is the queue of one audio source.
type mixerSource struct {
	mixer    *mixer
	queue    []byte
	lastRecv time.Time
	dropped  bool // not in mixer.sources anymore
	muted    bool // playback was cleared, ignore the rest of the source
}

// mixer sends the audio of one or more sources to the camera in chunks of
// exactly 40 ms on a 40 ms clock.
//
// FFmpeg delivers the RTP packets of a file or TTS playback with the size of
// the source frames (ex. 24 ms for MP3) and in bursts. A battery doorbell
// stutters if they are forwarded as is. Several sources are mixed, because
// the camera supports only one talk session.
type mixer struct {
	opts  mixerOptions
	write func(chunk []byte) error
	log   func(event string, stats *mixerStats)

	mu      sync.Mutex
	sources []*mixerSource

	done     chan struct{} // closed by Stop
	finished chan struct{} // closed when run has returned
	stopOnce sync.Once
}

// newMixer creates a mixer. Write gets one chunk of PCMA. Log is optional.
func newMixer(opts mixerOptions, write func(chunk []byte) error, log func(event string, stats *mixerStats)) *mixer {
	if opts.startBytes < mixerChunkSize {
		opts.startBytes = mixerChunkSize
	}
	if log == nil {
		log = func(string, *mixerStats) {}
	}
	return &mixer{
		opts:     opts,
		write:    write,
		log:      log,
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

// AddSource adds a queue for one more audio source.
func (m *mixer) AddSource() *mixerSource {
	src := &mixerSource{mixer: m}
	m.mu.Lock()
	m.sources = append(m.sources, src)
	m.mu.Unlock()
	return src
}

// Push queues the payload of the packet. It is the handler of core.Sender.
func (s *mixerSource) Push(packet *rtp.Packet) {
	m := s.mixer
	m.mu.Lock()
	defer m.mu.Unlock()

	if s.muted {
		return
	}
	if s.dropped {
		// the source continues after a long pause
		s.dropped = false
		m.sources = append(m.sources, s)
	}
	if len(s.queue) < mixerMaxQueue {
		s.queue = append(s.queue, packet.Payload...)
	}
	s.lastRecv = time.Now()
}

// Dropped reports that the source has finished and was removed.
func (s *mixerSource) Dropped() bool {
	s.mixer.mu.Lock()
	defer s.mixer.mu.Unlock()
	return s.dropped
}

// Clear drops the queued audio of all sources and ignores what they still
// send. Sources added later are played as usual.
func (m *mixer) Clear() {
	m.mu.Lock()
	for _, src := range m.sources {
		src.queue = nil
		src.muted = true
	}
	m.mu.Unlock()
}

// Stop sends the rest of the queued audio and stops the mixer. It waits not
// longer than the timeout.
func (m *mixer) Stop(timeout time.Duration) {
	m.stopOnce.Do(func() { close(m.done) })

	select {
	case <-m.finished:
	case <-time.After(timeout):
	}
}

// start runs the sender loop after the delay and the setup of the output.
// The sources can be added and filled at once. A Stop during the delay ends
// the mixer only if nothing is queued.
func (m *mixer) start(delay time.Duration, setup func() error) {
	go func() {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()

			done := m.done
		wait:
			for {
				select {
				case <-done:
					done = nil
					if m.empty() {
						close(m.finished)
						return
					}
				case <-timer.C:
					break wait
				}
			}
		}

		if err := setup(); err != nil {
			m.log("setup error: "+err.Error(), nil)
			close(m.finished)
			return
		}

		m.run()
	}()
}

// empty reports that no source has queued audio.
func (m *mixer) empty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, src := range m.sources {
		if len(src.queue) > 0 {
			return false
		}
	}
	return true
}

// next returns the next chunk of audio and how many sources are in it. It
// returns nil if no source has a full chunk. With flush also the rest of the
// queues is taken and filled up with silence.
func (m *mixer) next(flush bool) (chunk []byte, n int) {
	var parts [][]byte
	for _, src := range m.sources {
		size := len(src.queue)
		if size >= mixerChunkSize {
			size = mixerChunkSize
		} else if !flush || size == 0 {
			continue
		}
		parts = append(parts, src.queue[:size])
		src.queue = src.queue[size:]
	}
	if len(parts) == 0 {
		return nil, 0
	}
	return mixPCMA(parts, mixerChunkSize), len(parts)
}

// state collects the length of the longest queue and the time of the last
// packet, and drops sources that have finished.
func (m *mixer) state(now time.Time) (longest int, lastRecv time.Time) {
	for i := 0; i < len(m.sources); i++ {
		src := m.sources[i]
		if n := len(src.queue); n > longest {
			longest = n
		}
		if src.lastRecv.After(lastRecv) {
			lastRecv = src.lastRecv
		}
		if len(src.queue) == 0 && now.Sub(src.lastRecv) > mixerForget {
			src.dropped = true
			m.sources = append(m.sources[:i], m.sources[i+1:]...)
			i--
		}
	}
	return
}

// run is the sender loop. It returns after Stop or a write error.
func (m *mixer) run() {
	defer close(m.finished)

	ticker := time.NewTicker(mixerChunkTime)
	defer ticker.Stop()

	silence := make([]byte, mixerChunkSize)
	for i := range silence {
		silence[i] = pcmaSilence
	}

	var (
		playing  bool // between the pre-fill and the end of the audio
		stopping bool // Stop was called, send the rest and return
		leadLeft int  // silence chunks left before the audio
		tailLeft int  // silence chunks left after the audio
		gap      int  // ticks without audio since the last chunk
		lastSend time.Time
		stats    mixerStats
	)

	done := m.done

	for {
		select {
		case <-done:
			stopping = true
			done = nil
		case <-ticker.C:
		}

		now := time.Now()

		m.mu.Lock()
		longest, lastRecv := m.state(now)
		if longest > stats.MaxQueue {
			stats.MaxQueue = longest
		}

		// no more audio will come after Stop
		ended := stopping || now.Sub(lastRecv) >= 2*mixerChunkTime
		idle := stopping || now.Sub(lastRecv) >= mixerIdleEnd

		var chunk []byte
		var sources int

		switch {
		case !playing:
			// collect some audio before the start, but don't wait for more
			// than a source has
			if longest >= m.opts.startBytes || (longest > 0 && ended) {
				playing = true
				leadLeft = m.opts.leadChunks
				tailLeft = m.opts.prefillChunks
				stats = mixerStats{MaxQueue: longest}
				gap = 0
				lastSend = time.Time{}
				m.mu.Unlock()

				// The camera starts to play from an empty buffer. A cushion
				// of silence covers the jitter of the network.
				for i := 0; i < m.opts.prefillChunks; i++ {
					if err := m.write(silence); err != nil {
						m.log("write error: "+err.Error(), nil)
						return
					}
				}
				m.log("start", nil)
				continue
			}
		case leadLeft > 0:
			// the audio keeps queueing during the lead-in
		default:
			chunk, sources = m.next(longest < mixerChunkSize && ended)
		}
		m.mu.Unlock()

		if !playing {
			if stopping {
				return
			}
			continue
		}

		switch {
		case leadLeft > 0:
			leadLeft--
			chunk = silence
		case chunk != nil:
			stats.Chunks++
			if sources > 1 {
				stats.Mixed++
			}
			// an underrun is a pause in the middle of the audio
			stats.Underruns += gap
			if gap == 0 && !lastSend.IsZero() {
				d := now.Sub(lastSend)
				if d > stats.MaxGap {
					stats.MaxGap = d
				}
				if d > mixerChunkTime*3/2 {
					stats.Late++
				}
			}
			gap = 0
			lastSend = now
		case !idle:
			gap++
			continue
		case stopping && tailLeft > 0:
			// trailing silence, so the camera plays its buffer before the
			// session is closed
			tailLeft--
			chunk = silence
		default:
			m.log("finished", &stats)
			if stopping {
				return
			}
			playing = false
			continue
		}

		if err := m.write(chunk); err != nil {
			m.log("write error: "+err.Error(), nil)
			return
		}
		if d := time.Since(now); d > stats.MaxWrite {
			stats.MaxWrite = d
		}
	}
}

// mixPCMA mixes the parts into one chunk of the given size. Short parts are
// filled up with silence. One part is returned without a change of the audio.
func mixPCMA(parts [][]byte, size int) []byte {
	chunk := make([]byte, size)

	if len(parts) == 1 {
		n := copy(chunk, parts[0])
		for i := n; i < size; i++ {
			chunk[i] = pcmaSilence
		}
		return chunk
	}

	sum := make([]int32, size)
	for _, part := range parts {
		for i, b := range part {
			if i == size {
				break
			}
			sum[i] += int32(pcm.PCMAtoPCM(b))
		}
	}
	for i, v := range sum {
		chunk[i] = pcm.PCMtoPCMA(limitPCM(v, len(parts)))
	}
	return chunk
}

// limitPCM fits the sum of n sources into 16 bit. The level is not changed
// below the knee, so quiet sources sound the same as alone. Above the knee
// the range up to the largest possible sum is compressed into the headroom.
func limitPCM(v int32, n int) int16 {
	neg := v < 0
	if neg {
		v = -v
	}
	if v > mixerKnee {
		peak := int32(n) * 32768
		v = mixerKnee + (v-mixerKnee)*(32767-mixerKnee)/(peak-mixerKnee)
	}
	if neg {
		v = -v
	}
	return int16(v)
}
