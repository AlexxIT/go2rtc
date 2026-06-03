package tapo

import (
	"bytes"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/rs/zerolog/log"
	"github.com/pion/rtp"
)

// backchannelFramesPerPart controls how many 20 ms PCMA frames are bundled
// into a single multipart HTTP part sent to the camera.  Sending one frame
// per part (the naive approach) causes the camera to treat each HTTP boundary
// as a discrete audio burst, producing an audible "beep-beep-beep" pattern.
// Five frames (100 ms) matches the chunk size that produces continuous audio.
const backchannelFramesPerPart = 5

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

		// Accumulate backchannelFramesPerPart RTP packets before flushing as
		// one multipart part.  Sending one packet per part produced "beep" artefacts
		// because the camera parsed each HTTP boundary as a separate audio burst.
		var (
			partBuf   []byte
			count     int
			lastSend  time.Time
			chunkNum  int
		)

		log.Info().Msg("tapo backchannel: audio forwarding active")
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			partBuf = append(partBuf, muxer.GetPayload(pid, packet.Timestamp, packet.Payload)...)
			count++
			if count >= backchannelFramesPerPart {
				now := time.Now()
				if !lastSend.IsZero() {
					interval := now.Sub(lastSend).Milliseconds()
					if interval < 80 || interval > 120 {
						log.Warn().Int64("interval_ms", interval).Int("chunk", chunkNum).Msg("tapo backchannel timing jitter")
					}
				}
				lastSend = now
				chunkNum++
				if err := c.WriteBackchannel(partBuf); err != nil {
					log.Warn().Err(err).Msg("tapo backchannel write error")
				}
				partBuf = partBuf[:0]
				count = 0
			}
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) SetupBackchannel() (err error) {
	// Battery-powered cameras (e.g. D230, DB200, H100) sleep after ~30s without
	// an active preview stream. Ensure preview is running on conn1 first so the
	// camera stays awake. Handle() discards frames when no video receivers are
	// registered, so there is no behavioural change for video+audio consumers.
	if c.session1 == "" {
		if err = c.SetupStream(); err != nil {
			return
		}
	}

	if c.conn2, err = c.newConn(); err != nil {
		return
	}

	c.session2, err = c.Request(c.conn2, []byte(`{"params":{"talk":{"mode":"aec"},"method":"get"},"seq":3,"type":"request"}`))
	return
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
