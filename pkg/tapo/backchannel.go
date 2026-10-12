package tapo

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

// Logger is used for the diagnostics of the backchannel. It is silent by
// default, go2rtc sets it to the logger of the module.
var Logger = zerolog.Nop()

// The camera needs a moment to release the talk session of a previous
// connection and rejects a new one with error_code=-52411.
const (
	talkBusyError   = "-52411"
	talkBusyRetries = 3
	talkBusyDelay   = 700 * time.Millisecond
)

// backchannelFramesPerPart controls how many 20 ms PCMA frames are bundled
// into a single multipart HTTP part sent to the camera. Sending one frame
// per part causes the camera to treat each HTTP boundary as a discrete audio
// burst, producing an audible "beep-beep-beep" pattern.
const backchannelFramesPerPart = 2

// backchannelPrefillChunks is the default number of silence chunks sent
// immediately before the audio. Without pre-fill the camera starts playing
// from an empty buffer and any jitter between the chunks causes audible gaps.
const backchannelPrefillChunks = 2

// Optional query params of the source URL, see README.
const (
	// pre-fill in milliseconds, 0..2000, default 80
	backchannelPrefillParam = "backchannel_prefill"
	// enables paced mode, value is how many milliseconds of audio are
	// collected before the first chunk is sent, 0..2000
	backchannelPaceParam = "backchannel_pace"
	// paced mode: silence in milliseconds sent in real time before the
	// audio, 0..5000
	backchannelLeadInParam = "backchannel_leadin"
	// paced mode: milliseconds between the wake up of a battery camera and
	// the talk session, 0..15000
	backchannelWakeParam = "backchannel_wake"
)

// backchannelStopTimeout limits how long Stop waits for the queued audio,
// plus the delays before the audio.
const backchannelStopTimeout = 10 * time.Second

// backchannelOptions are parsed from the query of the source URL.
type backchannelOptions struct {
	paced bool
	wake  time.Duration // time between the wake up and the talk session
	mixerOptions
}

func parseBackchannelOptions(query url.Values) (opts backchannelOptions) {
	const chunkMs = backchannelFramesPerPart * 20

	param := func(name string, max int) (int, bool) {
		if s := query.Get(name); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v >= 0 && v <= max {
				return v, true
			}
		}
		return 0, false
	}

	opts.prefillChunks = backchannelPrefillChunks
	if ms, ok := param(backchannelPrefillParam, 2000); ok {
		opts.prefillChunks = ms / chunkMs
	}
	if ms, ok := param(backchannelPaceParam, 2000); ok {
		opts.paced = true
		opts.startBytes = ms * 8
	}
	if ms, ok := param(backchannelLeadInParam, 5000); ok {
		opts.leadChunks = ms / chunkMs
	}
	if ms, ok := param(backchannelWakeParam, 15000); ok {
		opts.wake = time.Duration(ms) * time.Millisecond
	}
	return
}

// backchannelSender is a sender of the paced backchannel with its queue.
type backchannelSender struct {
	sender *core.Sender
	source *mixerSource
}

// CanMix reports that several sources can be played at the same time.
func (c *Client) CanMix() bool {
	return parseBackchannelOptions(c.url.Query()).paced
}

// ClearPlayback drops the audio that is queued for the camera.
func (c *Client) ClearPlayback() {
	c.backMu.Lock()
	defer c.backMu.Unlock()

	if c.mixer != nil {
		c.mixer.Clear()
	}
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	c.backMu.Lock()
	defer c.backMu.Unlock()

	if c.sender == nil && c.mixer == nil {
		opts := parseBackchannelOptions(c.url.Query())

		muxer := mpegts.NewMuxer()
		pid := muxer.AddTrack(mpegts.StreamTypePCMATapo)

		setup := func() error {
			if err := c.SetupBackchannel(); err != nil {
				return err
			}
			Logger.Debug().Bool("paced", opts.paced).Msg("[tapo] backchannel: talk session started")
			return c.WriteBackchannel(muxer.GetHeader())
		}

		// A battery doorbell (D235) freezes for 2-3 seconds about 4 seconds
		// after the wake up if a talk session is active at this time. So the
		// talk session is started only when the camera is awake long enough,
		// the audio waits in the mixer.
		var delay time.Duration
		if opts.paced && opts.wake > 0 && c.awake != nil {
			if d := c.sinceWake(); d < opts.wake {
				delay = opts.wake - d
			}
		}

		if delay > 0 {
			// the preview keeps the camera awake
			if err := c.SetupStream(); err != nil {
				return err
			}
			Logger.Debug().Dur("delay", delay).Msg("[tapo] backchannel: camera just woke up, talk session delayed")
		} else if err := setup(); err != nil {
			return err
		}

		if opts.paced {
			// muxer ignores the step after timestamp 0
			ts := uint32(mixerChunkSize)
			c.mixer = newMixer(opts.mixerOptions, func(chunk []byte) error {
				err := c.WriteBackchannel(muxer.GetPayload(pid, ts, chunk))
				ts += mixerChunkSize
				return err
			}, logMixer)
			c.stopTimeout = backchannelStopTimeout + delay + time.Duration(opts.leadChunks)*mixerChunkTime

			if delay > 0 {
				c.mixer.start(delay, setup)
			} else {
				c.mixer.start(0, func() error { return nil })
			}
		} else {
			c.sender = core.NewSender(media, track.Codec)
			c.sender.Handler = c.directBackchannel(muxer, pid, opts.prefillChunks)
		}
	}

	if c.mixer != nil {
		// forget the senders of the finished sources
		senders := c.backSenders[:0]
		for _, bs := range c.backSenders {
			if !bs.source.Dropped() {
				senders = append(senders, bs)
			}
		}

		// every source has its own queue, the mixer plays them together
		source := c.mixer.AddSource()
		sender := core.NewSender(media, track.Codec)
		sender.Handler = source.Push
		sender.HandleRTP(track)

		c.backSenders = append(senders, backchannelSender{sender, source})
		Logger.Debug().Int("sources", len(c.backSenders)).Msg("[tapo] backchannel: source added")
		return nil
	}

	c.sender.HandleRTP(track)
	return nil
}

// directBackchannel returns the handler that forwards the packets of a live
// source (two way audio) with the lowest latency: two packets per chunk, as
// they come.
func (c *Client) directBackchannel(muxer *mpegts.Muxer, pid uint16, prefillChunks int) func(packet *rtp.Packet) {
	silence := bytes.Repeat([]byte{pcmaSilence}, mixerChunkSize)

	var (
		rawBuf    []byte
		firstTS   uint32
		count     int
		lastSend  time.Time
		prefilled bool
	)

	return func(packet *rtp.Packet) {
		// Pre-fill on first packet using timestamps that flow into real audio,
		// avoiding PTS discontinuities that cause decoder resets.
		if !prefilled {
			prefilled = true
			for i := uint32(prefillChunks); i > 0; i-- {
				ts := packet.Timestamp - i*mixerChunkSize
				_ = c.WriteBackchannel(muxer.GetPayload(pid, ts, silence))
			}
		}

		if count == 0 {
			firstTS = packet.Timestamp
			rawBuf = rawBuf[:0]
		}
		rawBuf = append(rawBuf, packet.Payload...)
		count++

		if count < backchannelFramesPerPart {
			return
		}
		count = 0

		// all frames as one PES packet
		chunk := muxer.GetPayload(pid, firstTS, rawBuf)

		now := time.Now()
		if !lastSend.IsZero() {
			if d := now.Sub(lastSend); d < mixerChunkTime*4/5 || d > mixerChunkTime*6/5 {
				Logger.Trace().Dur("interval", d).Msg("[tapo] backchannel: timing jitter")
			}
		}
		lastSend = now

		if err := c.WriteBackchannel(chunk); err != nil {
			Logger.Debug().Err(err).Msg("[tapo] backchannel: write error")
		}
	}
}

func logMixer(event string, stats *mixerStats) {
	e := Logger.Debug()
	if stats != nil {
		e = e.Int("chunks", stats.Chunks).Int("mixed", stats.Mixed).
			Int("underruns", stats.Underruns).Int("late", stats.Late).
			Int("max_queue_ms", stats.MaxQueue/8).
			Int64("max_interval_ms", stats.MaxGap.Milliseconds()).
			Int64("max_write_ms", stats.MaxWrite.Milliseconds())
	}
	e.Msg("[tapo] backchannel: " + event)
}

// stopBackchannel closes the senders and lets the mixer send the rest of
// its queue.
func (c *Client) stopBackchannel() {
	c.backMu.Lock()
	sender, senders, mix, timeout := c.sender, c.backSenders, c.mixer, c.stopTimeout
	c.backMu.Unlock()

	if sender != nil {
		sender.Close()
	}
	for _, bs := range senders {
		bs.sender.Close()
	}
	if mix != nil {
		mix.Stop(timeout)
	}
}

func (c *Client) SetupBackchannel() (err error) {
	// Battery powered cameras (ex. D230, D235) sleep after ~30 s without an
	// active preview stream. So the preview always runs on conn1 and the
	// talk session gets its own connection. Handle drops the packets if
	// there are no receivers.
	if err = c.SetupStream(); err != nil {
		return
	}

	for i := 0; ; i++ {
		if c.conn2, err = c.newConn(); err != nil {
			return
		}

		c.session2, err = c.Request(c.conn2, []byte(`{"params":{"talk":{"mode":"aec"},"method":"get"},"seq":3,"type":"request"}`))
		if err == nil {
			return
		}

		_ = c.conn2.Close()

		if i == talkBusyRetries || !strings.Contains(err.Error(), talkBusyError) {
			return
		}

		Logger.Debug().Int("try", i+1).Msg("[tapo] backchannel: talk session busy, retry")
		time.Sleep(talkBusyDelay)
	}
}

func (c *Client) WriteBackchannel(body []byte) (err error) {
	buf := bytes.NewBuffer(make([]byte, 0, 256+len(body)))
	buf.WriteString("----client-stream-boundary--\r\n")
	buf.WriteString("Content-Type: audio/mp2t\r\n")
	buf.WriteString("X-If-Encrypt: 0\r\n")
	buf.WriteString("X-Session-Id: " + c.session2 + "\r\n")
	buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	buf.Write(body)

	_, err = buf.WriteTo(c.conn2)
	return
}
