package rtmp

import (
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/rtmp"
	"github.com/rs/zerolog"
)

func Init() {
	var conf struct {
		Mod struct {
			Listen string `yaml:"listen" json:"listen"`
		} `yaml:"rtmp"`
	}

	app.LoadConfig(&conf)

	log = app.GetLogger("rtmp")

	streams.HandleFunc("rtmp", streamsHandle)
	streams.HandleFunc("rtmps", streamsHandle)
	streams.HandleFunc("rtmpx", streamsHandle)

	api.HandleFunc("api/stream.flv", apiHandle)

	streams.HandleConsumerFunc("rtmp", streamsConsumerHandle)
	streams.HandleConsumerFunc("rtmps", streamsConsumerHandle)
	streams.HandleConsumerFunc("rtmpx", streamsConsumerHandle)

	address := conf.Mod.Listen
	if address == "" {
		return
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	log.Info().Str("addr", address).Msg("[rtmp] listen")

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func() {
				if err = tcpHandle(conn); err != nil {
					log.Error().Err(err).Caller().Send()
				}
			}()
		}
	}()
}

func tcpHandle(netConn net.Conn) error {
	rtmpConn, err := rtmp.NewServer(netConn)
	if err != nil {
		return err
	}

	if err = rtmpConn.ReadCommands(); err != nil {
		return err
	}

	switch rtmpConn.Intent {
	case rtmp.CommandPlay:
		stream := streams.Get(rtmpConn.App)
		if stream == nil {
			return errors.New("stream not found: " + rtmpConn.App)
		}

		cons := flv.NewConsumer()
		if err = stream.AddConsumer(cons); err != nil {
			return err
		}

		defer stream.RemoveConsumer(cons)

		if err = rtmpConn.WriteStart(); err != nil {
			return err
		}

		_, _ = cons.WriteTo(rtmpConn)

		return nil

	case rtmp.CommandPublish:
		stream := streams.Get(rtmpConn.App)
		if stream == nil {
			return errors.New("stream not found: " + rtmpConn.App)
		}

		if err = rtmpConn.WriteStart(); err != nil {
			return err
		}

		prod, err := rtmpConn.Producer()
		if err != nil {
			return err
		}

		stream.AddProducer(prod)

		defer stream.RemoveProducer(prod)

		_ = prod.Start()

		return nil
	}

	return errors.New("rtmp: unknown command: " + rtmpConn.Intent)
}

var log zerolog.Logger

func streamsHandle(url string) (core.Producer, error) {
	return rtmp.DialPlay(url)
}

func streamsConsumerHandle(url string) (core.Consumer, func(), error) {
	cons := flv.NewConsumer()
	run := func() {
		wr, err := rtmp.DialPublish(url)
		if err != nil {
			return
		}
		_, err = cons.WriteTo(wr)
	}

	return cons, run, nil
}

func apiHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		outputFLV(w, r)
	} else {
		inputFLV(w, r)
	}
}

func outputFLV(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := flv.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	h := w.Header()
	h.Set("Content-Type", "video/x-flv")

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)
}

func inputFLV(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	client, err := flv.Open(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.AddProducer(client)

	if err = client.Start(); err != nil && err != io.EOF {
		log.Warn().Err(err).Caller().Send()
	}

	stream.RemoveProducer(client)
}
