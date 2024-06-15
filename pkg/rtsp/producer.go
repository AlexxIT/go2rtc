package rtsp

import (
	"encoding/json"
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	core.Assert(media.Direction == core.DirectionRecvonly)

	for _, track := range c.Receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state == StatePlay {
		if err := c.Reconnect(); err != nil {
			return nil, err
		}
	}

	channel, err := c.SetupMedia(media)
	if err != nil {
		return nil, err
	}

	c.state = StateSetup

	track := core.NewReceiver(media, codec)
	track.ID = channel
	c.Receivers = append(c.Receivers, track)

	return track, nil
}

func (c *Conn) Start() (err error) {
	core.Assert(c.mode == core.ModeActiveProducer || c.mode == core.ModePassiveProducer)

	for {
		ok := false

		c.stateMu.Lock()
		switch c.state {
		case StateNone:
			err = nil
		case StateConn:
			err = errors.New("start from CONN state")
		case StateSetup:
			switch c.mode {
			case core.ModeActiveProducer:
				err = c.Play()
			case core.ModePassiveProducer:
				err = nil
			default:
				err = errors.New("start from wrong mode: " + c.mode.String())
			}

			if err == nil {
				c.state = StatePlay
				ok = true
			}
		}
		c.stateMu.Unlock()

		if !ok {
			return
		}

		// Handler can return different states:
		// 1. None after PLAY should exit without error
		// 2. Play after PLAY should exit from Start with error
		// 3. Setup after PLAY should Play once again
		err = c.Handle()
	}
}

func (c *Conn) Stop() (err error) {
	for _, receiver := range c.Receivers {
		receiver.Close()
	}
	for _, sender := range c.Senders {
		sender.Close()
	}

	c.stateMu.Lock()
	if c.state != StateNone {
		c.state = StateNone
		err = c.Close()
	}
	c.stateMu.Unlock()

	return
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Connection)
}

func (c *Conn) Reconnect() error {
	c.Fire("RTSP reconnect")

	// close current session
	_ = c.Close()

	// start new session
	if err := c.Dial(); err != nil {
		return err
	}
	if err := c.Describe(); err != nil {
		return err
	}

	// restore previous medias
	for _, receiver := range c.Receivers {
		if _, err := c.SetupMedia(receiver.Media); err != nil {
			return err
		}
	}
	for _, sender := range c.Senders {
		if _, err := c.SetupMedia(sender.Media); err != nil {
			return err
		}
	}

	return nil
}
