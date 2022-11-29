package rtsp

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog"
	"net"
	"strings"
)

func Init() {
	var conf struct {
		Mod struct {
			Listen   string `yaml:"listen"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"rtsp"`
	}

	// default config
	conf.Mod.Listen = ":8554"

	app.LoadConfig(&conf)

	log = app.GetLogger("rtsp")

	// RTSP client support
	streams.HandleFunc("rtsp", rtspHandler)
	streams.HandleFunc("rtsps", rtspHandler)
	streams.HandleFunc("rtspx", rtspHandler)

	// RTSP server support
	address := conf.Mod.Listen
	if address == "" {
		return
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Error().Err(err).Msg("[rtsp] listen")
		return
	}

	_, Port, _ = net.SplitHostPort(address)

	log.Info().Str("addr", address).Msg("[rtsp] listen")

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			c := rtsp.NewServer(conn)
			// skip check auth for localhost
			if conf.Mod.Username != "" && !conn.RemoteAddr().(*net.TCPAddr).IP.IsLoopback() {
				c.Auth(conf.Mod.Username, conf.Mod.Password)
			}
			go tcpHandler(c)
		}
	}()
}

type Handler func(conn *rtsp.Conn) bool

func HandleFunc(handler Handler) {
	handlers = append(handlers, handler)
}

var Port string

// internal

var log zerolog.Logger
var handlers []Handler

func rtspHandler(url string) (streamer.Producer, error) {
	backchannel := true

	if i := strings.IndexByte(url, '#'); i > 0 {
		if url[i+1:] == "backchannel=0" {
			backchannel = false
		}
		url = url[:i]
	}

	conn, err := rtsp.NewClient(url)
	if err != nil {
		return nil, err
	}

	conn.UserAgent = app.UserAgent

	if log.Trace().Enabled() {
		conn.Listen(func(msg interface{}) {
			switch msg := msg.(type) {
			case *tcp.Request:
				log.Trace().Msgf("[rtsp] client request:\n%s", msg)
			case *tcp.Response:
				log.Trace().Msgf("[rtsp] client response:\n%s", msg)
			}
		})
	}

	if err = conn.Dial(); err != nil {
		return nil, err
	}

	conn.Backchannel = backchannel
	if err = conn.Describe(); err != nil {
		if !backchannel {
			return nil, err
		}

		// second try without backchannel, we need to reconnect
		conn.Backchannel = false
		if err = conn.Dial(); err != nil {
			return nil, err
		}
		if err = conn.Describe(); err != nil {
			return nil, err
		}
	}

	return conn, nil
}

func tcpHandler(conn *rtsp.Conn) {
	var name string
	var closer func()

	trace := log.Trace().Enabled()

	conn.Listen(func(msg interface{}) {
		if trace {
			switch msg := msg.(type) {
			case *tcp.Request:
				log.Trace().Msgf("[rtsp] server request:\n%s", msg)
			case *tcp.Response:
				log.Trace().Msgf("[rtsp] server response:\n%s", msg)
			}
		}

		switch msg {
		case rtsp.MethodDescribe:
			if len(conn.URL.Path) == 0 {
				log.Warn().Msg("[rtsp] server empty URL on DESCRIBE")
				return
			}

			name = conn.URL.Path[1:]

			stream := streams.Get(name)
			if stream == nil {
				return
			}

			log.Debug().Str("stream", name).Msg("[rtsp] new consumer")

			initMedias(conn)

			if err := stream.AddConsumer(conn); err != nil {
				log.Warn().Err(err).Str("stream", name).Msg("[rtsp]")
				return
			}

			closer = func() {
				stream.RemoveConsumer(conn)
			}

		case rtsp.MethodAnnounce:
			if len(conn.URL.Path) == 0 {
				log.Warn().Msg("[rtsp] server empty URL on ANNOUNCE")
				return
			}

			name = conn.URL.Path[1:]

			stream := streams.Get(name)
			if stream == nil {
				return
			}

			log.Debug().Str("stream", name).Msg("[rtsp] new producer")

			stream.AddProducer(conn)

			closer = func() {
				stream.RemoveProducer(conn)
			}

		case streamer.StatePlaying:
			log.Debug().Str("stream", name).Msg("[rtsp] start")
		}
	})

	if err := conn.Accept(); err != nil {
		log.Warn().Err(err).Caller().Send()
		_ = conn.Close()
		return
	}

	for _, handler := range handlers {
		if handler(conn) {
			return
		}
	}

	if closer != nil {
		if err := conn.Handle(); err != nil {
			log.Debug().Err(err).Caller().Send()
		}

		closer()

		log.Debug().Str("stream", name).Msg("[rtsp] disconnect")
	}

	_ = conn.Close()
}

func initMedias(conn *rtsp.Conn) {
	// set media candidates from query list
	for key, value := range conn.URL.Query() {
		switch key {
		case streamer.KindVideo, streamer.KindAudio:
			for _, name := range value {
				name = strings.ToUpper(name)

				// check aliases
				switch name {
				case "COPY":
					name = "" // pass empty codecs list
				case "MJPEG":
					name = streamer.CodecJPEG
				case "AAC":
					name = streamer.CodecAAC
				}

				media := &streamer.Media{
					Kind: key, Direction: streamer.DirectionRecvonly,
				}

				// empty codecs match all codecs
				if name != "" {
					// empty clock rate and channels match any values
					media.Codecs = []*streamer.Codec{{Name: name}}
				}

				conn.Medias = append(conn.Medias, media)
			}
		}
	}

	// set default media candidates if query is empty
	if conn.Medias == nil {
		conn.Medias = []*streamer.Media{
			{Kind: streamer.KindVideo, Direction: streamer.DirectionRecvonly},
			{Kind: streamer.KindAudio, Direction: streamer.DirectionRecvonly},
		}
	}
}
