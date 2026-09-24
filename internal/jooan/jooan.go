package jooan

import (
	"errors"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/jooan"
)

type AliasConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	UID            string `yaml:"uid"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	SpeakerChannel uint32 `yaml:"speaker_channel"`
	LoginTimeout   int    `yaml:"login_timeout"`
}

var aliases map[string]AliasConfig

func Init() {
	var cfg struct {
		Cfg map[string]AliasConfig `yaml:"jooan"`
	}
	app.LoadConfig(&cfg)
	aliases = cfg.Cfg

	streams.HandleFunc("jooan", handle)
}

func handle(source string) (core.Producer, error) {
	const prefix = "jooan:"
	if !strings.HasPrefix(source, prefix) {
		return nil, errors.New("jooan: source must be an alias")
	}
	name := source[len(prefix):]
	if strings.HasPrefix(name, "//") {
		name = name[2:]
	}
	if strings.Contains(name, ":") || strings.ContainsAny(name, "/?#@") {
		return nil, errors.New("jooan: source must be an alias")
	}
	if name == "" {
		return nil, errors.New("jooan: alias is required")
	}
	cfg, ok := aliases[name]
	if !ok {
		return nil, errors.New("jooan: alias not found")
	}
	return jooan.NewProducer(jooan.Config{
		Host: cfg.Host, Port: cfg.Port, UID: cfg.UID, Username: cfg.Username,
		Password: cfg.Password, SpeakerChannel: cfg.SpeakerChannel,
		LoginTimeout: cfg.LoginTimeout,
	})
}
