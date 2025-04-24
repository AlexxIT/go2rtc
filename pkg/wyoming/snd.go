package wyoming

import (
	"net"
	"time"
)

func (s *Server) HandleSnd(conn net.Conn) {
	defer conn.Close()

	var snd []byte

	api := NewAPI(conn)
	for {
		evt, err := api.ReadEvent()
		if err != nil {
			return
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
				s.Error("snd error: %s", err)
				return
			}
		}
	}
}
