package pipe

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"strings"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver == nil {
		c.receiver = core.NewReceiver(media, codec)
	}
	return c.receiver, nil
}

func (c *Client) Start() error {
	return c.handle()
}

func (c *Client) Stop() (err error) {
	if c.receiver != nil {
		c.receiver.Close()
	}
	if err1 := c.stdout.Close(); err != nil {
		err = err1
	}
	if err1 := c.cmd.Process.Kill(); err != nil {
		err = err1
	}
	if err1 := c.cmd.Wait(); err != nil {
		err = err1
	}
	return
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:   "PIPE active producer",
		URL:    c.cmd.Path + " " + strings.Join(c.cmd.Args, " "),
		Medias: c.medias,
		Recv:   c.recv,
	}
	if c.receiver != nil {
		info.Receivers = append(info.Receivers, c.receiver)
	}
	return json.Marshal(info)
}
