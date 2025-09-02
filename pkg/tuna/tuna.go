package tuna

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Tuna struct {
	core.Listener

	Tunnels map[string]string

	reader *bufio.Reader
}

type Message struct {
	Msg   string `json:"msg"`
	Addr  string `json:"addr"`
	URL   string `json:"url"`
	Level string `json:"level"`
	Line  string
}

func NewTuna(command any) (*Tuna, error) {
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

	n := &Tuna{
		Tunnels: map[string]string{},
		reader:  bufio.NewReader(r),
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *Tuna) Serve() error {
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

		if msg.Level != "error" && strings.HasPrefix(msg.Msg, "Forwarding") {
			parts := strings.Fields(msg.Msg)
			if len(parts) >= 3 {
				url := parts[1]
				addr := parts[3]
				msg.URL = url
				msg.Addr = addr
				n.Tunnels[addr] = url
			}
			fmt.Println(msg.Addr, msg.URL)
		}

		msg.Line = string(line)

		n.Fire(msg)
	}
}
