package reolink

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Client struct {
	core.Listener

	url     *url.URL
	bc      *baichuan.Client
	stream  baichuan.Stream
	channel uint8

	medias    []*core.Media
	receivers []*core.Receiver
	sender    *core.Sender

	reader *baichuan.MediaReader
	cancel context.CancelFunc

	talkSession *baichuan.TalkSession
	talkEncoder *baichuan.ADPCMEncoder
	pcmBuf      []int16

	recv    int
	send    int

	probeIFrame *baichuan.MediaPacket

	videoTimestamps timestampUnwrapper
	videoRTP        rtpTimestampGuard

	audioSamples uint64
	audioRTP     rtpTimestampGuard

	baseTime       uint64
	systemBaseTime uint64
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	c := &Client{url: u}

	// parsing url
	cfg := baichuan.Config{
		Timeout: core.ConnDialTimeout,
	}

	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	host := u.Hostname()
	if strings.Contains(host, ".") || host == "localhost" {
		cfg.Host = host
	} else if len(host) >= 15 { // UID usually long
		cfg.UID = host
	} else {
		cfg.Host = host
	}

	if portStr := u.Port(); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = p
		}
	}

	// parsing stream name
	c.stream = baichuan.StreamMain
	path := strings.TrimPrefix(u.Path, "/")
	if strings.EqualFold(path, "sub") || strings.EqualFold(path, "substream") {
		c.stream = baichuan.StreamSub
	} else if strings.EqualFold(path, "extern") || strings.EqualFold(path, "ext") {
		c.stream = baichuan.StreamExtern
	}

	// channel
	c.channel = 0
	if chStr := u.Query().Get("channel"); chStr != "" {
		if ch, err := strconv.Atoi(chStr); err == nil {
			c.channel = uint8(ch)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), core.ConnDialTimeout)
	defer cancel()

	bc, err := baichuan.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := bc.Login(ctx); err != nil {
		bc.Close()
		return nil, err
	}

	c.bc = bc

	return c, nil
}

func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.reader != nil {
		c.reader.Close()
	}
	if c.talkSession != nil {
		ctx, cancel := context.WithTimeout(context.Background(), core.ConnDialTimeout)
		_ = c.talkSession.Close(ctx)
		cancel()
	}
	if c.bc != nil {
		return c.bc.Close()
	}
	return nil
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if c.sender == nil {
		if err := c.SetupBackchannel(); err != nil {
			return err
		}

		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *core.Packet) {
			payload := packet.Payload
			n := len(payload)
			for i := 0; i < n; i += 2 {
				lo := int16(payload[i])
				hi := int16(payload[i+1])
				sample := (hi << 8) | lo
				c.pcmBuf = append(c.pcmBuf, sample)
			}

			samplesPerBlock := c.talkSession.SamplesPerBlock()
			for len(c.pcmBuf) >= samplesPerBlock {
				block := c.pcmBuf[:samplesPerBlock]
				c.pcmBuf = c.pcmBuf[samplesPerBlock:]
				
				if adpcmBlock, err := c.talkEncoder.EncodeBlock(block); err == nil {
					_ = c.talkSession.WriteADPCMBlock(context.Background(), adpcmBlock)
					c.send += len(adpcmBlock)
				}
			}
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) SetupBackchannel() error {
	ctx, cancel := context.WithTimeout(context.Background(), core.ConnDialTimeout)
	defer cancel()

	session, err := c.bc.StartTalk(ctx, c.channel)
	if err != nil {
		return err
	}
	c.talkSession = session
	c.talkEncoder = &baichuan.ADPCMEncoder{}
	return nil
}
