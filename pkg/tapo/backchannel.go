package tapo

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"strconv"
)

func (c *Client) backchannelWriter() streamer.WriterFunc {
	w := mpegts.NewWriter()
	w.AddPES(68, mpegts.StreamTypePCMATapo)
	w.WritePAT()
	w.WritePMT()

	return func(packet *rtp.Packet) (err error) {
		// don't know why 68 and 192
		w.WritePES(68, 192, packet.Payload)
		err = c.WriteBackchannel(w.Bytes())
		w.Reset()
		return
	}
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
