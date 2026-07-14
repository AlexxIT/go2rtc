package isapi

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
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

	mu     sync.Mutex
	ffCmd  *exec.Cmd
	ffIn   io.WriteCloser
	ffDone chan struct{}
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
			{
				Name:      core.CodecAAC,
				ClockRate: c.sampleRate,
				Channels:  1,
				FmtpLine:  aac.FMTP + hex.EncodeToString(conf),
			},
			// WebRTC mic offers Opus/PCMU/PCMA. Match G.711 and transcode to AAC.
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

	switch {
	case c.codecName == core.CodecAAC && track.Codec.Name == core.CodecAAC:
		c.sender.Handler = func(packet *rtp.Packet) {
			c.writeADTSFrames(packet.Payload)
		}
		if track.Codec.IsRTP() {
			c.sender.Handler = aac.RTPToADTS(codec, c.sender.Handler)
		} else {
			c.sender.Handler = aac.EncodeToADTS(codec, c.sender.Handler)
		}

	case c.codecName == core.CodecAAC && (track.Codec.Name == core.CodecPCMU || track.Codec.Name == core.CodecPCMA):
		srcCodec := track.Codec.Name
		c.sender.Handler = func(packet *rtp.Packet) {
			c.writePCMUToAAC(srcCodec, packet.Payload)
		}

	default:
		c.sender.Handler = func(packet *rtp.Packet) {
			if c.conn == nil {
				return
			}
			c.send += len(packet.Payload)
			_, _ = c.conn.Write(packet.Payload)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) Start() (err error) {
	return c.Open()
}

func (c *Client) Stop() (err error) {
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
			return
		}
		if _, err := c.conn.Write(frame); err != nil {
			return
		}
		c.send += 4 + size
	}
}

func (c *Client) writePCMUToAAC(codecName string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	if err := c.ensureFFmpeg(codecName); err != nil {
		return
	}
	c.mu.Lock()
	in := c.ffIn
	c.mu.Unlock()
	if in == nil {
		return
	}
	_, _ = in.Write(payload)
}

func (c *Client) ensureFFmpeg(codecName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ffCmd != nil {
		return nil
	}

	sampleFmt := "mulaw"
	if codecName == core.CodecPCMA {
		sampleFmt = "alaw"
	}

	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", sampleFmt, "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		"-c:a", "aac", "-profile:a", "aac_low",
		"-ar", strconv.Itoa(int(c.sampleRate)), "-ac", "1", "-b:a", "64k",
		"-f", "adts", "pipe:1",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	c.ffCmd = cmd
	c.ffIn = stdin
	c.ffDone = make(chan struct{})

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
	done := c.ffDone
	c.ffCmd = nil
	c.ffIn = nil
	c.ffDone = nil
	c.mu.Unlock()

	if in != nil {
		_ = in.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if done != nil {
		<-done
	}
}
