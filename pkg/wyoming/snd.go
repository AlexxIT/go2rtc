package wyoming

import (
	"io"
	"net"
	"time"
)

func (s *Server) HandleSnd(conn net.Conn) error {
	defer conn.Close()

	var snd []byte

	api := NewAPI(conn)
	for {
		evt, err := api.ReadEvent()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		s.Trace("event: %s data: %s payload: %d", evt.Type, evt.Data, len(evt.Payload))

		switch evt.Type {
		case "audio-start":
			snd = snd[:0]
		case "audio-chunk":
			snd = append(snd, evt.Payload...)
		case "audio-stop":
			prod := newSndProducer(snd, func() {
				time.Sleep(time.Second) // some extra delay before close
			})
			if err = s.SndHandler(prod); err != nil {
				return err
			}
		}
	}
}
