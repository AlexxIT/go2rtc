package doorbird

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const (
	AudioMixerInterval    = 10 * time.Millisecond
	AudioChannelBuffer    = 10
	OutputChannelBuffer   = 10
	SenderCleanupInterval = 5 * time.Second
	SenderTimeoutDuration = 5 * time.Second
)

var (
	cltMu  sync.Mutex
	cltMap = make(map[string]*Client)
)

type AudioMixer struct {
	mu      sync.Mutex
	streams map[string]chan []byte
	output  chan []byte
	running bool
	closed  bool
}

func NewAudioMixer() *AudioMixer {
	return &AudioMixer{
		streams: make(map[string]chan []byte),
		output:  make(chan []byte, OutputChannelBuffer),
	}
}

func (m *AudioMixer) AddStream(id string) chan []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		ch := make(chan []byte)
		close(ch)
		return ch
	}

	if !m.running {
		m.running = true
		go m.mixLoop()
	}

	stream := make(chan []byte, AudioChannelBuffer)
	m.streams[id] = stream
	return stream
}

func (m *AudioMixer) RemoveStream(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stream, exists := m.streams[id]; exists {
		close(stream)
		delete(m.streams, id)
	}
}

func (m *AudioMixer) mixLoop() {
	ticker := time.NewTicker(AudioMixerInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}

		if len(m.streams) == 0 {
			m.mu.Unlock()
			continue
		}

		var pcmSamples [][]int16
		activeStreams := 0

		for _, stream := range m.streams {
			select {
			case data := <-stream:
				if len(data) > 0 {
					samples := make([]int16, len(data))
					for i, sample := range data {
						samples[i] = pcm.PCMUtoPCM(sample)
					}
					pcmSamples = append(pcmSamples, samples)
					activeStreams++
				}
			default:
			}
		}
		m.mu.Unlock()

		if activeStreams == 0 {
			continue
		}

		var mixedLength int
		for _, samples := range pcmSamples {
			if len(samples) > mixedLength {
				mixedLength = len(samples)
			}
		}

		if mixedLength == 0 {
			continue
		}

		mixed := make([]int16, mixedLength)
		for i := 0; i < mixedLength; i++ {
			var sum int32
			var count int32

			for _, samples := range pcmSamples {
				if i < len(samples) {
					sum += int32(samples[i])
					count++
				}
			}

			if count > 0 {
				averaged := sum / count
				if averaged > 32767 {
					mixed[i] = 32767
				} else if averaged < -32768 {
					mixed[i] = -32768
				} else {
					mixed[i] = int16(averaged)
				}
			}
		}

		output := make([]byte, len(mixed))
		for i, sample := range mixed {
			output[i] = pcm.PCMtoPCMU(sample)
		}

		select {
		case m.output <- output:
		default:
		}
	}
}

type Client struct {
	core.Connection
	conn        net.Conn
	mixer       *AudioMixer
	trackMap    map[*core.Sender]string
	senderStats map[*core.Sender]time.Time
	mu          sync.RWMutex
}

func Dial(rawURL string) (*Client, error) {
	cltMu.Lock()
	defer cltMu.Unlock()

	if existingClient, exists := cltMap[rawURL]; exists && existingClient.conn != nil {
		return existingClient, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	user := u.User.Username()
	pass, _ := u.User.Password()

	if u.Port() == "" {
		u.Host += ":80"
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	s := fmt.Sprintf("POST /bha-api/audio-transmit.cgi?http-user=%s&http-password=%s HTTP/1.0\r\n", user, pass) +
		"Content-Type: audio/basic\r\n" +
		"Content-Length: 9999999\r\n" +
		"Connection: Keep-Alive\r\n" +
		"Cache-Control: no-cache\r\n" +
		"\r\n"

	_ = conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
	if _, err = conn.Write([]byte(s)); err != nil {
		conn.Close()
		return nil, err
	}

	resp, _ := http.ReadResponse(bufio.NewReader(conn), nil)
	if resp != nil {
		switch resp.StatusCode {
		case 204:
			conn.Close()
			return nil, errors.New("DoorBird user has no api permission")
		case 503:
			conn.Close()
			return nil, errors.New("DoorBird device is busy")
		}
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCMU, ClockRate: 8000},
			},
		},
	}

	client := &Client{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "doorbird",
			Protocol:   "http",
			URL:        rawURL,
			Medias:     medias,
		},
		conn:        conn,
		mixer:       NewAudioMixer(),
		trackMap:    make(map[*core.Sender]string),
		senderStats: make(map[*core.Sender]time.Time),
	}

	cltMap[rawURL] = client

	return client, nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sender := core.NewSender(media, track.Codec)
	trackID := fmt.Sprintf("%d", core.NewID())
	streamChan := c.mixer.AddStream(trackID)

	sender.Handler = func(pkt *rtp.Packet) {
		if len(pkt.Payload) == 0 {
			return
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn != nil {
			select {
			case streamChan <- pkt.Payload:
				c.mu.Lock()
				c.senderStats[sender] = time.Now()
				c.mu.Unlock()
			default:
			}
		}
	}

	c.trackMap[sender] = trackID
	c.senderStats[sender] = time.Now()

	if len(c.Senders) == 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
				}
			}()

			for mixedData := range c.mixer.output {
				c.mu.RLock()
				conn := c.conn
				c.mu.RUnlock()

				if conn != nil && len(mixedData) > 0 {
					_ = conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
					if n, err := conn.Write(mixedData); err == nil {
						c.Send += n
					} else {
						break
					}
				}
			}
		}()
	}

	sender.WithParent(track).Start()
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) Start() error {
	if c.conn == nil {
		return nil
	}

	go func() {
		ticker := time.NewTicker(SenderCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanupOrphanedSenders()
		}
	}()

	buf := make([]byte, 1)
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, err := c.conn.Read(buf)
		if err != nil {
			c.cleanup()
			cltMu.Lock()
			delete(cltMap, c.URL)
			cltMu.Unlock()
			return err
		}
	}
}

func (c *Client) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	if c.mixer != nil {
		c.mixer.mu.Lock()
		c.mixer.closed = true
		for id, stream := range c.mixer.streams {
			close(stream)
			delete(c.mixer.streams, id)
		}
		if c.mixer.running {
			close(c.mixer.output)
			c.mixer.running = false
		}
		c.mixer.mu.Unlock()
	}

	for _, sender := range c.Senders {
		sender.Close()
	}
	c.Senders = nil

	c.trackMap = make(map[*core.Sender]string)
	c.senderStats = make(map[*core.Sender]time.Time)

	cltMu.Lock()
	delete(cltMap, c.URL)
	cltMu.Unlock()
}

func (c *Client) cleanupOrphanedSenders() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removedCount := 0
	validIndex := 0

	for i, sender := range c.Senders {
		lastActivity, exists := c.senderStats[sender]
		if sender.State() == "closed" || !exists || now.Sub(lastActivity) >= SenderTimeoutDuration {
			if trackID, exists := c.trackMap[sender]; exists {
				c.mixer.RemoveStream(trackID)
				delete(c.trackMap, sender)
			}
			delete(c.senderStats, sender)
			sender.Close()
			removedCount++
		} else {
			c.Senders[validIndex] = c.Senders[i]
			validIndex++
		}
	}

	c.Senders = c.Senders[:validIndex]

	if removedCount > 0 {
		fmt.Printf("DoorBird: Cleaned up %d orphaned senders, %d remain active\n", removedCount, validIndex)
	}
}

func (c *Client) RemoveTrack(sender *core.Sender) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if trackID, exists := c.trackMap[sender]; exists {
		c.mixer.RemoveStream(trackID)
		delete(c.trackMap, sender)
	}
	delete(c.senderStats, sender)

	for i, s := range c.Senders {
		if s == sender {
			c.Senders = append(c.Senders[:i], c.Senders[i+1:]...)
			break
		}
	}
}
