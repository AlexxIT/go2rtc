package stdin

import (
	"os/exec"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Deprecated: should be rewritten to core.Connection
type Client struct {
	cmd *exec.Cmd

	medias []*core.Media
	sender *core.Sender
	send   int
}

func NewClient(cmd *exec.Cmd) (*Client, error) {
	c := &Client{
		cmd: cmd,
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
