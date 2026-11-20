package unifi

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/unifi"
)

func Init() {
	var cfg struct {
		FFmpeg struct {
			Bin string `yaml:"bin"`
		} `yaml:"ffmpeg"`
	}

	cfg.FFmpeg.Bin = "ffmpeg"

	app.LoadConfig(&cfg)

	log := app.GetLogger("unifi")

	streams.HandleFunc("unifi", func(source string) (core.Producer, error) {
		client, err := unifi.Dial(source)
		if err != nil {
			return nil, err
		}

		client.Backchannel = func(session *unifi.Session, codec *core.Codec) (core.Producer, error) {
			log.Debug().Str("codec", session.Codec).Int("rate", session.SamplingRate).Msg("[unifi] talkback session")
			return newBackchannel(cfg.FFmpeg.Bin, session, codec)
		}

		return client, nil
	})

	// hide api_key of configured sources before they are dialed
	for _, sources := range streams.GetAllSources() {
		for _, source := range sources {
			if s, ok := strings.CutPrefix(source, "unifi:"); ok {
				if u, err := url.Parse(s); err == nil {
					creds.AddSecret(u.Query().Get("api_key"))
				}
			}
		}
	}
}

// newBackchannel - FFmpeg via exec module, so `exec.allow_paths` applies and FFmpeg stops with the producer
func newBackchannel(bin string, session *unifi.Session, codec *core.Codec) (core.Producer, error) {
	cmd, err := backchannelCommand(bin, session, codec)
	if err != nil {
		return nil, err
	}
	return streams.GetProducer(cmd)
}

func backchannelCommand(bin string, session *unifi.Session, codec *core.Codec) (string, error) {
	output, err := talkbackURL(session.URL)
	if err != nil {
		return "", err
	}

	if !strings.EqualFold(session.Codec, "opus") {
		return "", errors.New("unifi: unsupported talkback codec: " + session.Codec)
	}

	var input, audio string
	switch codec.Name {
	case core.CodecPCMA:
		input, audio = "-f alaw -ar 8000 -ac 1 -i -", "alaw/8000"
	case core.CodecPCMU:
		input, audio = "-f mulaw -ar 8000 -ac 1 -i -", "mulaw/8000"
	default:
		return "", errors.New("unifi: unsupported microphone codec: " + codec.Name)
	}

	rate := strconv.Itoa(session.SamplingRate)

	return "exec:" + bin + " -hide_banner -v error -fflags nobuffer " + input +
		" -c:a libopus -application:a lowdelay -ar:a " + rate + " -ac:a 1 -f rtp " + output +
		"#backchannel=1#audio=" + audio, nil
}

// talkbackURL - the URL becomes part of the FFmpeg command line, query params there are protocol options,
// so the URL is rebuilt from scheme, host and port only
func talkbackURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "rtp" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return "", errors.New("unifi: unsupported talkback url")
	}

	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 || host == "" || strings.ContainsAny(host, " \"'#{}?&") {
		return "", errors.New("unifi: unsupported talkback url")
	}

	return u.Scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)), nil
}
