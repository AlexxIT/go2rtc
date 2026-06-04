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

// backchannelPrefillChunks is the number of silence chunks sent immediately
// when the first real RTP packet arrives.  Without pre-fill the camera
// starts playing from an empty buffer; any scheduling jitter between chunks
// (even <1 ms) causes underruns and audible gaps.  Sending N×100 ms of
// silence before real audio gives the camera a cushion to absorb jitter.
const backchannelPrefillChunks = 3

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

		// PCMA silence: 0xD5 is G.711 A-law encoding of zero amplitude.
		silenceFrame := make([]byte, 160*backchannelFramesPerPart)
		for i := range silenceFrame {
			silenceFrame[i] = 0xD5
		}

		var (
			rawBuf    []byte
			firstTS   uint32
			count     int
			lastSend  time.Time
			chunkNum  int
			prefilled bool
		)

		log.Debug().Msg("tapo backchannel: audio forwarding active")
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			// Pre-fill on first packet using timestamps that flow into real audio,
			// avoiding PTS discontinuities that cause decoder resets.
			if !prefilled {
				prefilled = true
				step := uint32(backchannelFramesPerPart * 160)
				for i := uint32(backchannelPrefillChunks); i > 0; i-- {
					ts := packet.Timestamp - i*step
					_ = c.WriteBackchannel(muxer.GetPayload(pid, ts, silenceFrame))
				}
				log.Debug().Int("chunks", backchannelPrefillChunks).Msg("tapo backchannel: pre-fill sent")
			}

			if count == 0 {
				firstTS = packet.Timestamp
				rawBuf = rawBuf[:0]
			}
			rawBuf = append(rawBuf, packet.Payload...)
			count++

			if count >= backchannelFramesPerPart {
				// Wrap all frames as ONE PES packet, matching test_tapo_backchannel.py
				chunk := muxer.GetPayload(pid, firstTS, rawBuf)
				now := time.Now()
				if !lastSend.IsZero() {
					interval := now.Sub(lastSend).Milliseconds()
					expected := int64(backchannelFramesPerPart * 20)
					if interval < expected*4/5 || interval > expected*6/5 {
						log.Warn().Int64("interval_ms", interval).Int64("expected_ms", expected).Int("chunk", chunkNum).Msg("tapo backchannel timing jitter")
					}
				}
				lastSend = now
				chunkNum++
				if err := c.WriteBackchannel(chunk); err != nil {
					log.Warn().Err(err).Msg("tapo backchannel write error")
				}
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
