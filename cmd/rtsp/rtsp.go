package rtsp

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog"
	"net"
)

func Init() {
	var conf struct {
		Mod struct {
			Listen string `yaml:"listen"`
		} `yaml:"rtsp"`
	}

	// default config
	conf.Mod.Listen = ":8554"

	app.LoadConfig(&conf)

	log = app.GetLogger("rtsp")

	// RTSP client support
	streams.HandleFunc("rtsp", rtspHandler)
	streams.HandleFunc("rtsps", rtspHandler)

	// RTSP server support
	address := conf.Mod.Listen
	if address != "" {
		_, Port, _ = net.SplitHostPort(address)

		go worker(address)
	}
}

var Port string

var OnProducer func(conn streamer.Producer) bool // TODO: maybe rewrite...

// internal

var log zerolog.Logger

func rtspHandler(url string) (streamer.Producer, error) {
	conn, err := rtsp.NewClient(url)
	if err != nil {
		return nil, err
	}

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
	if err = conn.Describe(); err != nil {
		return nil, err
	}

	return conn, nil
}

func worker(address string) {
	srv, err := tcp.NewServer(address)
	if err != nil {
		log.Error().Err(err).Msg("[rtsp] listen")
		return
	}

	log.Info().Str("addr", address).Msg("[rtsp] listen")

	srv.Listen(func(msg interface{}) {
		switch msg.(type) {
		case net.Conn:
			var name string
			var onDisconnect func()

			trace := log.Trace().Enabled()

			conn := rtsp.NewServer(msg.(net.Conn))
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
					name = conn.URL.Path[1:]

					log.Debug().Str("stream", name).Msg("[rtsp] new consumer")

					stream := streams.Get(name) // TODO: rewrite
					if stream == nil {
						return
					}

					initMedias(conn)

					if err = stream.AddConsumer(conn); err != nil {
						log.Warn().Err(err).Str("stream", name).Msg("[rtsp]")
						return
					}

					onDisconnect = func() {
						stream.RemoveConsumer(conn)
					}

				case rtsp.MethodAnnounce:
					if OnProducer != nil {
						if OnProducer(conn) {
							return
						}
					}

					name = conn.URL.Path[1:]

					log.Debug().Str("stream", name).Msg("[rtsp] new producer")

					str := streams.Get(conn.URL.Path[1:])
					if str == nil {
						return
					}

					str.AddProducer(conn)

					onDisconnect = func() {
						str.RemoveProducer(conn)
					}

				case streamer.StatePlaying:
					log.Debug().Str("stream", name).Msg("[rtsp] start")
				}
			})

			if err = conn.Accept(); err != nil {
				log.Warn().Err(err).Msg("[rtsp] accept")
				return
			}

			if err = conn.Handle(); err != nil {
				//log.Warn().Err(err).Msg("[rtsp] handle server")
			}

			if onDisconnect != nil {
				onDisconnect()
			}

			log.Debug().Str("stream", name).Msg("[rtsp] disconnect")
		}
	})

	srv.Serve()
}

func initMedias(conn *rtsp.Conn) {
	// set media candidates from query list
	for key, value := range conn.URL.Query() {
		switch key {
		case streamer.KindVideo, streamer.KindAudio:
			for _, value := range value {
				media := &streamer.Media{
					Kind: key, Direction: streamer.DirectionRecvonly,
				}

				switch value {
				case "", "copy": // pass empty codecs list
				default:
					codec := streamer.NewCodec(value)
					media.Codecs = append(media.Codecs, codec)
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
