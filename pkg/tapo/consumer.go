package tapo

import (
	"bytes"
	"strconv"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
)

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if c.sender == nil {
		if err := c.SetupBackchannel(); err != nil {
			return err
		}

		muxer := mpegts.NewMuxer()
		pid := muxer.AddTrack(mpegts.StreamTypePCMATapo)
		if err := c.WriteBackchannel(muxer.GetHeader()); err != nil {
			return err
		}

		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			b := muxer.GetPayload(pid, packet.Timestamp, packet.Payload)
			_ = c.WriteBackchannel(b)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) SetupBackchannel() (err error) {
	// if conn1 is not used - we will use it for backchannel
	// or we need to start another conn for session2
	if c.session1 != "" {
		if c.conn2, err = c.newConn(); err != nil {
			return
		}
	} else {
		c.conn2 = c.conn1
	}

	c.session2, err = c.Request(c.conn2, []byte(`{"params":{"talk":{"mode":"aec"},"method":"get"},"seq":3,"type":"request"}`))
	return
}

func (c *Client) WriteBackchannel(body []byte) (err error) {
	// TODO: fixme (size)
	buf := bytes.NewBuffer(nil)
	buf.WriteString("----client-stream-boundary--\r\n")
	buf.WriteString("Content-Type: audio/mp2t\r\n")
	buf.WriteString("X-If-Encrypt: 0\r\n")
	buf.WriteString("X-Session-Id: " + c.session2 + "\r\n")
	buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	buf.Write(body)

	_, err = buf.WriteTo(c.conn2)
	return
}
