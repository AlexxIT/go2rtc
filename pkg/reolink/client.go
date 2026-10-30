package reolink

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

var (
	keepaliveInterval = 10 * time.Second
	// idleTimeout ends the talk session after this much silence, so the camera is free for the app
	idleTimeout = 2 * time.Second
	// retryDelay prevents a new TalkConfig for every RTP packet after a refusal
	retryDelay = 3 * time.Second
	// maxAhead limits how much audio may wait in the camera, for sources faster than real time
	maxAhead = time.Second
)

// blocksPerTalk is the number of ADPCM blocks per Talk message (256 ms at 16 kHz)
const blocksPerTalk = 4

// silenceLevel is the loudest sample that still counts as digital silence. A muted browser
// microphone keeps sending silence, and A-law has no exact zero (its quietest step decodes to 8).
const silenceLevel = 16

// Client is a talk-only producer, to be used as a second source next to the camera's video stream
type Client struct {
	core.Connection

	// OnError reports talk errors, which happen outside any call that could return them
	OnError func(error) `json:"-"`

	conn    *conn
	channel int
	cfg     talkParams

	// copies of the package timings, taken at Dial
	idleTimeout time.Duration
	maxAhead    time.Duration

	mu       sync.Mutex
	talking  bool
	session  uint32
	encoder  adpcmEncoder
	samples  []int16
	lastPush time.Time
	playEnd  time.Time // when the camera has played everything sent so far
	idle     *time.Timer
	retryAt  time.Time

	stopped  atomic.Bool
	stopOnce sync.Once
	done     chan struct{}
}

// Dial logs in and asks the camera which audio it accepts.
// Format: reolink://user:pass@192.168.1.123[:9000][?channel=0]
func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	address := u.Host
	if u.Port() == "" {
		address = net.JoinHostPort(u.Hostname(), "9000")
	}

	var channel int
	if s := u.Query().Get("channel"); s != "" {
		// message channel bytes 1-100 address channels 0-99
		if channel, err = strconv.Atoi(s); err != nil || channel < 0 || channel > 99 {
			return nil, fmt.Errorf("reolink: wrong channel %q", s)
		}
	}

	conn, err := dial(address, 3*keepaliveInterval)
	if err != nil {
		return nil, err
	}

	pass, _ := u.User.Password()
	if err = conn.login(u.User.Username(), pass); err != nil {
		_ = conn.Close()
		return nil, err
	}

	msg, err := conn.request(cmdTalkAbility, byte(channel+1), channelExt(channel), "")
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	cfg, err := parseTalkAbility(conn.decryptXML(msg))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    pcm.ProducerCodecs(),
		},
	}

	c := &Client{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "reolink",
			Protocol:   "tcp",
			RemoteAddr: address,
			URL:        rawURL,
			Medias:     medias,
			Transport:  conn,
		},
		conn:        conn,
		channel:     channel,
		cfg:         cfg,
		idleTimeout: idleTimeout,
		maxAhead:    maxAhead,
		done:        make(chan struct{}),
	}
	go c.keepalive(keepaliveInterval)
	return c, nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	rate := uint32(c.cfg.sampleRate)

	// 8 kHz from a browser is doubled with a filter, other rates go through pcm.Transcode
	var up *upsampler
	if src := track.Codec; src.ClockRate*2 == rate {
		rate = src.ClockRate
		up = &upsampler{}
	}
	toPCM := pcm.Transcode(&core.Codec{Name: core.CodecPCML, ClockRate: rate}, track.Codec)

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		b := toPCM(packet.Payload)
		samples := make([]int16, len(b)/2)
		for i := range samples {
			samples[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
		}
		if up != nil {
			samples = up.process(samples)
		}
		if err := c.push(samples, len(packet.Payload)); err != nil && c.OnError != nil {
			c.OnError(err)
		}
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) Start() error {
	<-c.conn.done
	return c.conn.closedErr()
}

// Stop returns at once, because streams calls it with the stream locked. The camera first plays
// what it already has, then the session is reset and the connection closed in the background.
func (c *Client) Stop() error {
	c.stopOnce.Do(func() {
		c.stopped.Store(true) // push and onIdle stop here, no lock: push may wait for the camera
		close(c.done)
		for _, sender := range c.Senders {
			sender.Close()
		}

		go func() {
			select {
			case <-c.conn.done: // dead connection: nothing to flush, reset or log out
				return
			default:
			}

			c.mu.Lock()
			talking := c.talking
			var wait time.Duration
			if talking {
				c.flush()
				wait = time.Until(c.playEnd) + 100*time.Millisecond
			}
			c.mu.Unlock()

			if wait > 0 {
				time.Sleep(min(wait, c.maxAhead+time.Second))
			}
			if talking {
				c.mu.Lock()
				c.endTalk(true)
				c.mu.Unlock()
			}

			c.conn.logout()
			c.conn.closeGracefully()
		}()
	})
	return nil
}

func (c *Client) keepalive(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.conn.write(cmdLinkType, c.conn.nextID(hostChannel), classModern, nil, nil); err != nil {
				return
			}
		case <-c.done:
			return
		case <-c.conn.done:
			return
		}
	}
}

// push opens a talk session on the first audio and sends full Talk messages
func (c *Client) push(samples []int16, size int) error {
	if c.stopped.Load() {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped.Load() {
		return nil
	}
	c.Send += size
	now := time.Now()
	quiet := silent(samples)
	if !quiet {
		c.lastPush = now
	}

	if !c.talking {
		// silence opens no session, so a muted microphone leaves the camera free
		if quiet || now.Before(c.retryAt) {
			return nil
		}
		if err := c.openTalk(); err != nil {
			c.retryAt = now.Add(retryDelay)
			return err
		}
	}

	// pauses in speech are sent, but after idleTimeout of silence the idle timer ends the session
	if quiet && now.Sub(c.lastPush) >= c.idleTimeout {
		return nil
	}

	c.samples = append(c.samples, samples...)

	n := c.cfg.samplesPerBlock * blocksPerTalk
	for len(c.samples) >= n {
		// a source faster than real time must not flood the camera
		if ahead := time.Until(c.playEnd); ahead > c.maxAhead {
			time.Sleep(ahead - c.maxAhead)
		}
		if err := c.sendTalk(c.samples[:n]); err != nil {
			c.talking = false
			c.samples = c.samples[:0]
			c.retryAt = time.Now().Add(retryDelay)
			return err
		}
		c.samples = append(c.samples[:0], c.samples[n:]...)
	}
	return nil
}

// openTalk sends TalkConfig. A 422 means an earlier session was not closed: reset and try once more.
func (c *Client) openTalk() error {
	ch := byte(c.channel + 1)
	ext := channelExt(c.channel)
	body := c.cfg.talkConfig(c.channel)

	_, err := c.conn.request(cmdTalkConfig, ch, ext, body)
	var se *statusError
	if errors.As(err, &se) && se.status == 422 {
		if _, err = c.conn.request(cmdTalkReset, ch, ext, ""); err != nil {
			return err
		}
		_, err = c.conn.request(cmdTalkConfig, ch, ext, body)
	}
	if err != nil {
		return err
	}

	c.talking = true
	c.session = c.conn.nextID(ch)
	c.encoder = adpcmEncoder{}
	c.samples = c.samples[:0]
	c.playEnd = time.Now()

	if c.idle == nil {
		c.idle = time.AfterFunc(c.idleTimeout, c.onIdle)
	} else {
		c.idle.Reset(c.idleTimeout)
	}
	return nil
}

// sendTalk encodes samples to ADPCM blocks; the media itself is not encrypted, only the extension
func (c *Client) sendTalk(samples []int16) error {
	n := c.cfg.samplesPerBlock
	var payload []byte
	for i := 0; i < len(samples); i += n {
		block := make([]int16, n)
		copy(block, samples[i:min(i+n, len(samples))])
		payload = appendFrame(payload, c.encoder.encodeBlock(block))
	}

	ext := c.conn.encrypt([]byte(talkExt(c.channel)))
	if err := c.conn.write(cmdTalk, c.session, classModern, ext, payload); err != nil {
		return err
	}

	blocks := (len(samples) + n - 1) / n
	duration := time.Duration(blocks*n) * time.Second / time.Duration(c.cfg.sampleRate)
	if now := time.Now(); c.playEnd.Before(now) {
		c.playEnd = now
	}
	c.playEnd = c.playEnd.Add(duration)
	return nil
}

func silent(samples []int16) bool {
	for _, s := range samples {
		if s > silenceLevel || s < -silenceLevel {
			return false
		}
	}
	return true
}

// onIdle ends the session once no sound arrived for idleTimeout and the camera played everything
func (c *Client) onIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.talking || c.stopped.Load() {
		return
	}
	if wait := c.idleTimeout - time.Since(c.lastPush); wait > 0 {
		c.idle.Reset(wait)
		return
	}
	c.flush()
	if wait := time.Until(c.playEnd) + 100*time.Millisecond; wait > 0 {
		c.idle.Reset(wait)
		return
	}
	c.endTalk(false)
}

// flush sends the remaining samples, padded with silence to whole blocks
func (c *Client) flush() {
	if len(c.samples) == 0 {
		return
	}
	if err := c.sendTalk(c.samples); err != nil && c.OnError != nil {
		c.OnError(err)
	}
	c.samples = c.samples[:0]
}

// endTalk resets the session. Only Stop waits for the reply (in the background), the idle timer
// does not, so a slow camera never holds up new audio.
func (c *Client) endTalk(wait bool) {
	c.talking = false
	ch := byte(c.channel + 1)

	var err error
	if wait {
		_, err = c.conn.request(cmdTalkReset, ch, channelExt(c.channel), "")
	} else {
		ext := c.conn.encrypt([]byte(channelExt(c.channel)))
		err = c.conn.write(cmdTalkReset, c.conn.nextID(ch), classModern, ext, nil)
	}
	if err != nil && c.OnError != nil {
		c.OnError(err)
	}
}
