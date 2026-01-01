package wyoming

import (
	"bytes"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
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
			prod := pcm.OpenSync(sndCodec, bytes.NewReader(snd))
			if err = s.SndHandler(prod); err != nil {
				s.Error("snd error: %s", err)
				return
			}
		}
	}
}

var sndCodec = &core.Codec{Name: core.CodecPCML, ClockRate: 22050}
