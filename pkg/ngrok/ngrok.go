package ngrok

import (
	"bufio"
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"io"
	"os/exec"
	"strings"
)

type Ngrok struct {
	streamer.Element

	Tunnels map[string]string

	reader *bufio.Reader
}

type Message struct {
	Msg  string `json:"msg"`
	Addr string `json:"addr"`
	URL  string `json:"url"`
	Line string
}

func NewNgrok(command interface{}) (*Ngrok, error) {
	var arg []string
	switch command.(type) {
	case string:
		arg = strings.Split(command.(string), " ")
	case []string:
		arg = command.([]string)
	}

	arg = append(arg, "--log", "stdout", "--log-format", "json")

	cmd := exec.Command(arg[0], arg[1:]...)

	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	n := &Ngrok{
		Tunnels: map[string]string{},
		reader:  bufio.NewReader(r),
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *Ngrok) Serve() error {
	for {
		line, _, err := n.reader.ReadLine()
		if err != nil {
			if err != io.EOF {
				return err
			}
			return nil
		}

		msg := new(Message)
		_ = json.Unmarshal(line, msg)

		if msg.Msg == "started tunnel" {
			n.Tunnels[msg.Addr] = msg.URL
		}

		msg.Line = string(line)

		n.Fire(msg)
	}
}
