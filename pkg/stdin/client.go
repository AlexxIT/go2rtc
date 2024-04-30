package stdin

import (
	"io"
	"os/exec"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Client struct {
	cmd  *exec.Cmd
	pipe io.WriteCloser

	medias []*core.Media
	sender *core.Sender
	send   int
}

func NewClient(cmd *exec.Cmd) (*Client, error) {
	pipe, err := PipeCloser(cmd)
	if err != nil {
		return nil, err
	}

	c := &Client{
		pipe: pipe,
		cmd:  cmd,
		medias: []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecPCMA, ClockRate: 8000},
					{Name: core.CodecPCM},
				},
			},
		},
	}

	return c, nil
}
