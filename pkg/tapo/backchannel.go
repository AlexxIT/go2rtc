package tapo

import (
	"bytes"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

// Logger is used for the diagnostics of the backchannel. It is silent by
// default, go2rtc sets it to the logger of the module.
var Logger = zerolog.Nop()

// The camera needs a moment to release the talk session of a previous
// connection and rejects a new one with error_code=-52411.
const (
	talkBusyError   = "-52411"
	talkBusyRetries = 3
	talkBusyDelay   = 700 * time.Millisecond
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
	// Battery powered cameras (ex. D230, D235) sleep after ~30 s without an
	// active preview stream. So the preview always runs on conn1 and the
	// talk session gets its own connection. Handle drops the packets if
	// there are no receivers.
	if err = c.SetupStream(); err != nil {
		return
	}

	for i := 0; ; i++ {
		if c.conn2, err = c.newConn(); err != nil {
			return
		}

		c.session2, err = c.Request(c.conn2, []byte(`{"params":{"talk":{"mode":"aec"},"method":"get"},"seq":3,"type":"request"}`))
		if err == nil {
			return
		}

		_ = c.conn2.Close()

		if i == talkBusyRetries || !strings.Contains(err.Error(), talkBusyError) {
			return
		}

		Logger.Debug().Int("try", i+1).Msg("[tapo] backchannel: talk session busy, retry")
		time.Sleep(talkBusyDelay)
	}
}

func (c *Client) WriteBackchannel(body []byte) (err error) {
	buf := bytes.NewBuffer(make([]byte, 0, 256+len(body)))
	buf.WriteString("----client-stream-boundary--\r\n")
	buf.WriteString("Content-Type: audio/mp2t\r\n")
	buf.WriteString("X-If-Encrypt: 0\r\n")
	buf.WriteString("X-Session-Id: " + c.session2 + "\r\n")
	buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	buf.Write(body)

	_, err = buf.WriteTo(c.conn2)
	return
}
