package execbc

import (
	"io"
	"net"
	"os/exec"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Client struct {
	medias      []*core.Media
	sender      *core.Sender
	conn        net.Conn
	send        int
	pipeCloser  io.WriteCloser
	commandArgs []string
	cmd         *exec.Cmd
}

var lock = &sync.Mutex{}
var singleInstance *Client

func NewClient(commandArgs []string) (*Client, error) {
	return &Client{commandArgs: commandArgs}, nil
}

func (c *Client) Dial() error {
	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000},
		},
	}

	c.medias = append(c.medias, media)

	cmdName := c.commandArgs[0]
	args := c.commandArgs[1:]
	cmd := *exec.Command(cmdName, args...)

	pipeCloser, error := PipeCloser(&cmd)
	if error != nil {
		return error
	}
	c.pipeCloser = pipeCloser
	c.cmd = &cmd
	return nil
}

func (c Client) Open() (err error) {
	c.cmd.Run()
	return
}

func (c Client) Close() (err error) {
	return c.pipeCloser.Close()
}
