package wyoming

import (
	"fmt"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (s *Server) HandleMic(conn net.Conn) {
	defer conn.Close()

	var closed core.Waiter
	var timestamp int

	api := NewAPI(conn)
	mic := newMicConsumer(func(chunk []byte) {
		data := fmt.Sprintf(`{"rate":16000,"width":2,"channels":1,"timestamp":%d}`, timestamp)
		evt := &Event{Type: "audio-chunk", Data: data, Payload: chunk}
		if err := api.WriteEvent(evt); err != nil {
			closed.Done(nil)
		}

		timestamp += len(chunk) / 2
	})
	mic.RemoteAddr = api.conn.RemoteAddr().String()

	if err := s.MicHandler(mic); err != nil {
		s.Error("mic error: %s", err)
		return
	}

	_ = closed.Wait()
	_ = mic.Stop()
}
