package outputbc

import (
	"io"
	"net"
	"os/exec"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Client struct {
	medias  []*core.Media
	sender  *core.Sender
	conn    net.Conn
	send    int
	cmd     exec.Cmd
	pipe    io.WriteCloser
	command []string
}

var lock = &sync.Mutex{}
var singleInstance *Client

func NewClient(command []string) (*Client, error) {
	return &Client{command: command}, nil
}

func (c *Client) Dial() {
	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000},
		},
	}

	c.medias = append(c.medias, media)
	if c.pipe == nil {
		cmdName := c.command[0]
		args := c.command[1:]
		c.cmd = *exec.Command(cmdName, args...)
		c.pipe, _ = c.cmd.StdinPipe()
	}
}

func (c *Client) Open() (err error) {
	c.cmd.Run()
	return
}

func (c *Client) Close() (err error) {
	c.pipe.Close()
	c.cmd.Process.Kill()
	return
}
