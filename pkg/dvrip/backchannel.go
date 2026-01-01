package dvrip

import (
	"encoding/binary"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Backchannel struct {
	core.Connection
	client *Client
}

func (c *Backchannel) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Backchannel) Start() error {
	if err := c.client.conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	b := make([]byte, 4096)
	for {
		if _, err := c.client.rd.Read(b); err != nil {
			return err
		}
	}
}

func (c *Backchannel) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if err := c.client.Talk(); err != nil {
		return err
	}

	const PacketSize = 320

	buf := make([]byte, 8+PacketSize)
	binary.BigEndian.PutUint32(buf, 0x1FA)

	switch track.Codec.Name {
	case core.CodecPCMU:
		buf[4] = 10
	case core.CodecPCMA:
		buf[4] = 14
	}

	//for i, rate := range sampleRates {
	//	if rate == track.Codec.ClockRate {
	//		buf[5] = byte(i) + 1
	//		break
	//	}
	//}
	buf[5] = 2 // ClockRate=8000

	binary.LittleEndian.PutUint16(buf[6:], PacketSize)

	var payload []byte

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		payload = append(payload, packet.Payload...)

		for len(payload) >= PacketSize {
			buf = append(buf[:8], payload[:PacketSize]...)
			if n, err := c.client.WriteCmd(OPTalkData, buf); err != nil {
				c.Send += n
			}

			payload = payload[PacketSize:]
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}
