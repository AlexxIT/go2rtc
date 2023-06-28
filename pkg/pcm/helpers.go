package pcm

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"sync"
)

func RepackBackchannel(handler core.HandlerFunc) core.HandlerFunc {
	var buf []byte
	var seq uint16

	// fix https://github.com/AlexxIT/go2rtc/issues/432
	var mu sync.Mutex

	return func(packet *rtp.Packet) {
		mu.Lock()

		buf = append(buf, packet.Payload...)
		if len(buf) < 1024 {
			mu.Unlock()
			return
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,               // should be true
				PayloadType:    packet.PayloadType, // will be owerwriten
				SequenceNumber: seq,
				Timestamp:      0, // should be always zero
				SSRC:           packet.SSRC,
			},
			Payload: buf[:1024],
		}

		buf = buf[1024:]
		seq++

		mu.Unlock()

		handler(pkt)
	}
}
