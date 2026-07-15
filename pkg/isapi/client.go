package isapi

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// Deprecated: should be rewritten to core.Connection
type Client struct {
	core.Listener

	url     string
	channel string
	conn    net.Conn

	codecName  string // core.CodecAAC / CodecPCMU / CodecPCMA
	sampleRate uint32 // AAC talk sample rate (Hz), typically 16000
	sessionID  string

	medias []*core.Media
	sender *core.Sender
	send   int

	mu      sync.Mutex
	ffCmd   *exec.Cmd
	ffIn    io.WriteCloser
	ffOgg   *oggwriter.OggWriter
	ffSrc   string // matched source codec for current ffmpeg session
	ffDone  chan struct{}
	inPkts  atomic.Uint64
	inBytes atomic.Uint64
	outFrm  atomic.Uint64
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	u.Scheme = "http"
	u.Path = ""

	client := &Client{url: u.String()}
	if err = client.Dial(); err != nil {
		return nil, err
	}
	return client, err
}

func (c *Client) Dial() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels"
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		tcp.Close(res)
		return errors.New(res.Status)
	}

	b, err := io.ReadAll(res.Body)
	tcp.Close(res)
	if err != nil {
		return err
	}

	xml := string(b)

	codec := core.Between(xml, `<audioCompressionType>`, `<`)
	switch codec {
	case "G.711ulaw":
		c.codecName = core.CodecPCMU
		c.sampleRate = 8000
	case "G.711alaw":
		c.codecName = core.CodecPCMA
		c.sampleRate = 8000
	case "AAC":
		c.codecName = core.CodecAAC
		c.sampleRate = 16000
		if s := core.Between(xml, `<audioSamplingRate>`, `<`); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				// Camera XML uses kHz (e.g. 16) not Hz.
				if v < 1000 {
					c.sampleRate = uint32(v) * 1000
				} else {
					c.sampleRate = uint32(v)
				}
			}
		}
	default:
		return errors.New("isapi: unsupported two-way codec: " + codec)
	}

	c.channel = core.Between(xml, `<id>`, `<`)

	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
	}

	if c.codecName == core.CodecAAC {
		conf := aac.EncodeConfig(aac.TypeAACLC, c.sampleRate, 1, false)
		media.Codecs = []*core.Codec{
			// Prefer Opus (WebRTC mic) → ffmpeg → AAC. Avoid PCMU mush when possible.
			{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
			{Name: core.CodecOpus, ClockRate: 48000},
			{
				Name:      core.CodecAAC,
				ClockRate: c.sampleRate,
				Channels:  1,
				FmtpLine:  aac.FMTP + hex.EncodeToString(conf),
			},
			// Fallback: same G.711 match as stock, then transcode to AAC.
			{Name: core.CodecPCMU, ClockRate: 8000},
			{Name: core.CodecPCMA, ClockRate: 8000},
		}
	} else {
		media.Codecs = []*core.Codec{{
			Name:      c.codecName,
			ClockRate: c.sampleRate,
		}}
	}

	c.medias = append(c.medias, media)
	Log.Info().
		Str("host", c.url).
		Str("cam_codec", c.codecName).
		Uint32("sample_rate", c.sampleRate).
		Str("channel", c.channel).
		Msg("[isapi] dial two-way")
	return nil
}

func (c *Client) Open() (err error) {
	// Hikvision may reject open if a previous session was not closed.
	if err = c.Close(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)

	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel
	req, err := http.NewRequest("PUT", link+"/open", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	b, _ := io.ReadAll(res.Body)
	tcp.Close(res)

	if res.StatusCode != http.StatusOK {
		return errors.New("isapi: open: " + res.Status)
	}

	c.sessionID = core.Between(string(b), `<sessionId>`, `<`)

	audioData := link + "/audioData"
	if c.sessionID != "" {
		audioData += "?sessionId=" + url.QueryEscape(c.sessionID)
	}

	ctx, pconn := tcp.WithConn()
	req, err = http.NewRequestWithContext(ctx, "PUT", audioData, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", "0")

	res, err = tcp.Do(req)
	if err != nil {
		return err
	}

	c.conn = *pconn

	buf := make([]byte, 1)
	_, _ = c.conn.Read(buf)

	tcp.Close(res)
	Log.Info().
		Str("session", c.sessionID).
		Str("cam_codec", c.codecName).
		Msg("[isapi] open audioData OK")
	return nil
}

func (c *Client) Close() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel
	req, err := http.NewRequest("PUT", link+"/close", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return err
	}

	tcp.Close(res)
	return nil
}

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if c.sender != nil {
		c.sender.HandleRTP(track)
		return nil
	}

	c.sender = core.NewSender(media, track.Codec)
	src := track.Codec.Name
	Log.Info().
		Str("src", src).
		Uint32("src_rate", track.Codec.ClockRate).
		Uint8("src_ch", track.Codec.Channels).
		Str("cam", c.codecName).
		Msg("[isapi] AddTrack")

	switch {
	case c.codecName == core.CodecAAC && track.Codec.Name == core.CodecAAC:
		c.sender.Handler = func(packet *rtp.Packet) {
			c.noteIn(packet)
			c.writeADTSFrames(packet.Payload)
		}
		if track.Codec.IsRTP() {
			c.sender.Handler = aac.RTPToADTS(codec, c.sender.Handler)
		} else {
			c.sender.Handler = aac.EncodeToADTS(codec, c.sender.Handler)
		}

	case c.codecName == core.CodecAAC && track.Codec.Name == core.CodecOpus:
		c.sender.Handler = func(packet *rtp.Packet) {
			c.noteIn(packet)
			c.writeOpusToAAC(packet)
		}

	case c.codecName == core.CodecAAC && (track.Codec.Name == core.CodecPCMU || track.Codec.Name == core.CodecPCMA):
		srcCodec := track.Codec.Name
		c.sender.Handler = func(packet *rtp.Packet) {
			c.noteIn(packet)
			c.writePCMUToAAC(srcCodec, packet.Payload)
		}

	default:
		// G.711 cam: raw bytes straight through (no ffmpeg).
		Log.Info().Str("path", "raw").Str("codec", src).Msg("[isapi] G.711 passthrough")
		c.sender.Handler = func(packet *rtp.Packet) {
			if c.conn == nil {
				return
			}
			c.noteIn(packet)
			c.send += len(packet.Payload)
			if _, err := c.conn.Write(packet.Payload); err != nil {
				Log.Debug().Err(err).Msg("[isapi] G.711 write")
			}
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) noteIn(packet *rtp.Packet) {
	n := c.inPkts.Add(1)
	c.inBytes.Add(uint64(len(packet.Payload)))
	if n == 1 || n%50 == 0 {
		Log.Debug().
			Uint64("in_pkts", n).
			Uint64("in_bytes", c.inBytes.Load()).
			Uint64("out_frames", c.outFrm.Load()).
			Int("sent_bytes", c.send).
			Str("ff_src", c.ffSrcSnapshot()).
			Msg("[isapi] talk stats")
	}
}

func (c *Client) ffSrcSnapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ffSrc
}

func (c *Client) Start() (err error) {
	return c.Open()
}

func (c *Client) Stop() (err error) {
	inPkts := c.inPkts.Load()
	outFrm := c.outFrm.Load()
	Log.Info().
		Uint64("in_pkts", inPkts).
		Uint64("in_bytes", c.inBytes.Load()).
		Uint64("out_frames", outFrm).
		Int("sent_bytes", c.send).
		Str("ff_src", c.ffSrcSnapshot()).
		Msg("[isapi] stop")

	c.stopFFmpeg()

	if c.sender != nil {
		c.sender.Close()
	}

	if c.conn != nil {
		_ = c.Close()
		return c.conn.Close()
	}

	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: "isapi",
		Protocol:   "http",
		Medias:     c.medias,
		Send:       c.send,
	}
	if c.conn != nil {
		info.RemoteAddr = c.conn.RemoteAddr().String()
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}

// writeADTSFrames writes Hikvision ISAPI AAC framing: [u32be len][ADTS]...
func (c *Client) writeADTSFrames(b []byte) {
	if c.conn == nil || len(b) == 0 {
		return
	}

	for len(b) >= aac.ADTSHeaderSize {
		if !aac.IsADTS(b) {
			b = b[1:]
			continue
		}
		size := int(aac.ReadADTSSize(b))
		if size < aac.ADTSHeaderSize || size > len(b) {
			return
		}
		frame := b[:size]
		b = b[size:]

		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(size))
		if _, err := c.conn.Write(hdr[:]); err != nil {
			Log.Debug().Err(err).Msg("[isapi] AAC len write")
			return
		}
		if _, err := c.conn.Write(frame); err != nil {
			Log.Debug().Err(err).Msg("[isapi] AAC frame write")
			return
		}
		c.send += 4 + size
		c.outFrm.Add(1)
	}
}

func (c *Client) writeOpusToAAC(packet *rtp.Packet) {
	if packet == nil || len(packet.Payload) == 0 {
		return
	}
	if err := c.ensureFFmpeg(core.CodecOpus, packet); err != nil {
		return
	}
	c.mu.Lock()
	ogg := c.ffOgg
	c.mu.Unlock()
	if ogg == nil {
		return
	}
	if err := ogg.WriteRTP(packet); err != nil {
		Log.Debug().Err(err).Msg("[isapi] opus ogg write")
	}
}

func (c *Client) writePCMUToAAC(codecName string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	if err := c.ensureFFmpeg(codecName, nil); err != nil {
		return
	}
	c.mu.Lock()
	in := c.ffIn
	c.mu.Unlock()
	if in == nil {
		return
	}
	if _, err := in.Write(payload); err != nil {
		Log.Debug().Err(err).Msg("[isapi] g711→aac write")
	}
}

// findFFmpeg returns a usable ffmpeg binary.
// Frigate's go2rtc process often does NOT have `ffmpeg` on PATH; the ffmpeg
// module is configured with something like /usr/lib/ffmpeg/7.0/bin/ffmpeg.
func findFFmpeg() (string, error) {
	candidates := []string{
		"ffmpeg",
		"/usr/lib/ffmpeg/7.0/bin/ffmpeg",
		"/usr/lib/ffmpeg/5.0/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/usr/bin/ffmpeg",
	}
	for _, bin := range candidates {
		path, err := exec.LookPath(bin)
		if err == nil {
			return path, nil
		}
		// Absolute paths: LookPath fails if not in PATH; check directly.
		if len(bin) > 0 && bin[0] == '/' {
			if st, err := os.Stat(bin); err == nil && !st.IsDir() {
				return bin, nil
			}
		}
	}
	return "", errors.New("isapi: ffmpeg not found (needed for Opus/PCMU/PCMA → AAC)")
}

func (c *Client) ensureFFmpeg(codecName string, firstOpus *rtp.Packet) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ffCmd != nil {
		return nil
	}

	bin, err := findFFmpeg()
	if err != nil {
		Log.Error().Err(err).Str("src", codecName).Msg("[isapi] ffmpeg missing")
		return err
	}

	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-probesize", "32",
		"-analyzeduration", "0",
	}

	switch codecName {
	case core.CodecOpus:
		args = append(args,
			"-f", "ogg", "-i", "pipe:0",
		)
	case core.CodecPCMU:
		args = append(args,
			"-f", "mulaw", "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		)
	case core.CodecPCMA:
		args = append(args,
			"-f", "alaw", "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		)
	default:
		return errors.New("isapi: unsupported ffmpeg input: " + codecName)
	}

	args = append(args,
		"-c:a", "aac", "-profile:a", "aac_low",
		"-ar", strconv.Itoa(int(c.sampleRate)), "-ac", "1", "-b:a", "64k",
		"-f", "adts",
		"-flush_packets", "1",
		"pipe:1",
	)

	cmd := exec.Command(bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		Log.Error().Err(err).Str("bin", bin).Msg("[isapi] ffmpeg start")
		return err
	}

	c.ffCmd = cmd
	c.ffIn = stdin
	c.ffSrc = codecName
	c.ffDone = make(chan struct{})

	if codecName == core.CodecOpus {
		_ = firstOpus
		ch := uint16(2)
		rate := uint32(48000)
		if c.sender != nil && c.sender.Codec != nil {
			if c.sender.Codec.ClockRate > 0 {
				rate = c.sender.Codec.ClockRate
			}
			if c.sender.Codec.Channels > 0 {
				ch = uint16(c.sender.Codec.Channels)
			}
		}
		ogg, err := oggwriter.NewWith(stdin, rate, ch)
		if err != nil {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			Log.Error().Err(err).Msg("[isapi] oggwriter")
			c.ffCmd = nil
			c.ffIn = nil
			c.ffDone = nil
			return err
		}
		c.ffOgg = ogg
		Log.Info().
			Str("bin", bin).
			Str("src", "OPUS").
			Uint32("opus_rate", rate).
			Uint16("opus_ch", ch).
			Uint32("aac_rate", c.sampleRate).
			Msg("[isapi] ffmpeg Opus→AAC started")
	} else {
		Log.Info().
			Str("bin", bin).
			Str("src", codecName).
			Uint32("aac_rate", c.sampleRate).
			Msg("[isapi] ffmpeg G.711→AAC started")
	}

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			Log.Warn().Str("ffmpeg", sc.Text()).Msg("[isapi] ffmpeg stderr")
		}
	}()

	go func() {
		defer close(c.ffDone)
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 2048)
		for {
			n, err := stdout.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				for {
					if len(buf) < aac.ADTSHeaderSize {
						break
					}
					if !aac.IsADTS(buf) {
						buf = buf[1:]
						continue
					}
					size := int(aac.ReadADTSSize(buf))
					if size < aac.ADTSHeaderSize || size > len(buf) {
						break
					}
					frame := append([]byte(nil), buf[:size]...)
					buf = buf[size:]
					c.writeADTSFrames(frame)
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					Log.Debug().Err(err).Msg("[isapi] ffmpeg stdout")
				}
				return
			}
		}
	}()

	return nil
}

func (c *Client) stopFFmpeg() {
	c.mu.Lock()
	cmd := c.ffCmd
	in := c.ffIn
	ogg := c.ffOgg
	done := c.ffDone
	src := c.ffSrc
	c.ffCmd = nil
	c.ffIn = nil
	c.ffOgg = nil
	c.ffDone = nil
	c.ffSrc = ""
	c.mu.Unlock()

	if ogg != nil {
		_ = ogg.Close()
	} else if in != nil {
		_ = in.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if done != nil {
		<-done
	}
	if src != "" {
		Log.Debug().Str("src", src).Msg("[isapi] ffmpeg stopped")
	}
}
