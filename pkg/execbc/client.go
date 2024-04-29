package execbc

import (
	"io"
	"net"
	"os/exec"

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

func NewClient(commandArgs []string) (*Client, error) {
	c := &Client{commandArgs: commandArgs}
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
		return nil, error
	}
	c.pipeCloser = pipeCloser
	c.cmd = &cmd
	return c, nil
}

func (c Client) Open() (err error) {
	c.cmd.Run()
	return
}

func (c Client) Close() (err error) {
	return c.pipeCloser.Close()
}
