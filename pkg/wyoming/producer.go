package wyoming

import (
	"net"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	api *API
}

func newProducer(conn net.Conn) *Producer {
	return &Producer{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "wyoming",
			Medias: []*core.Media{
				{
					Kind:      core.KindAudio,
					Direction: core.DirectionRecvonly,
					Codecs: []*core.Codec{
						{Name: core.CodecPCML, ClockRate: 16000},
					},
				},
			},
			Transport: conn,
		},
		NewAPI(conn),
	}
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
