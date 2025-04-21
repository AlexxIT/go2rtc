package wyoming

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	api *API
}

func (p *Producer) Start() error {
	var seq uint16
	var ts uint32

	for {
		evt, err := p.api.ReadEvent()
		if err != nil {
			return err
		}

		if evt.Type != "audio-chunk" {
			continue
		}

		p.Recv += len(evt.Payload)

		pkt := &core.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				SequenceNumber: seq,
				Timestamp:      ts,
			},
			Payload: evt.Payload,
		}
		p.Receivers[0].WriteRTP(pkt)

		seq++
		ts += uint32(len(evt.Payload) / 2)
	}
}
