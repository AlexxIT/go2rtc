package rtsp

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	core.Assert(media.Direction == core.DirectionRecvonly)

	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	switch c.state {
	case StateConn, StateSetup:
	default:
		return nil, fmt.Errorf("RTSP GetTrack from wrong state: %s", c.state)
	}

	channel, err := c.SetupMedia(media, true)
	if err != nil {
		return nil, err
	}

	track := core.NewReceiver(media, codec)
	track.ID = byte(channel)
	c.receivers = append(c.receivers, track)

	return track, nil
}

func (c *Conn) Start() error {
	switch c.mode {
	case core.ModeActiveProducer:
		if err := c.Play(); err != nil {
			return err
		}
	case core.ModePassiveProducer:
	default:
		return fmt.Errorf("start wrong mode: %d", c.mode)
	}

	if err := c.Handle(); c.state != StateNone {
		_ = c.conn.Close()
		return err
	}

	return nil
}

func (c *Conn) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	for _, sender := range c.senders {
		sender.Close()
	}
	return c.Close()
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:      "RTSP " + c.mode.String(),
		UserAgent: c.UserAgent,
		Medias:    c.Medias,
		Receivers: c.receivers,
		Senders:   c.senders,
		Recv:      c.recv,
		Send:      c.send,
	}

	if c.URL != nil {
		info.URL = c.URL.String()
	}
	if c.conn != nil {
		info.RemoteAddr = c.conn.RemoteAddr().String()
	}

	return json.Marshal(info)
}
