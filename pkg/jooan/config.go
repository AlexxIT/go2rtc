package jooan

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	DefaultPort         = 32761
	AudioFrameBytes     = 640
	AudioSampleRate     = 16000
	AudioChannels       = 1
	AudioFramePeriod    = 40
	AudioFrameTimestamp = AudioFramePeriod
)

// Config contains the local credentials required to connect to one CAM720.
// It is intentionally not serializable to a source URL.
type Config struct {
	Host           string `yaml:"host" json:"host"`
	Port           int    `yaml:"port" json:"port,omitempty"`
	UID            string `yaml:"uid" json:"-"`
	Username       string `yaml:"username" json:"-"`
	Password       string `yaml:"password" json:"-"`
	SpeakerChannel uint32 `yaml:"speaker_channel" json:"speaker_channel,omitempty"`
	LoginTimeout   int    `yaml:"login_timeout" json:"login_timeout,omitempty"`
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" || strings.ContainsAny(c.Host, "/?#@") {
		return errors.New("jooan: host is required and must be a host name or LAN address")
	}
	if c.Port != 0 && (c.Port < 1 || c.Port > 65535) {
		return fmt.Errorf("jooan: invalid port %d", c.Port)
	}
	if strings.TrimSpace(c.UID) == "" {
		return errors.New("jooan: uid is required")
	}
	if strings.TrimSpace(c.Username) == "" || c.Password == "" {
		return errors.New("jooan: username and password are required")
	}
	return nil
}

func (c Config) port() int {
	if c.Port == 0 {
		return DefaultPort
	}
	return c.Port
}

func (c Config) speakerChannel() uint32 {
	if c.SpeakerChannel == 0 {
		return SpeakerChannel
	}
	return c.SpeakerChannel
}

func (c Config) String() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.port()))
}
