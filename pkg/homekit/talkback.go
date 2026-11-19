package homekit

import (
	"encoding/binary"
	"io"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

// HomeKit cameras and doorbells take talk-back audio in the codec selected for
// the stream, in both directions. Usually that is only AAC-ELD, for which
// go2rtc has no encoder (FDK AAC is not GPL compatible), so the encoder comes
// from outside, see internal/homekit.

// NewELDEncoder starts an AAC-ELD encoder for mono 16-bit little-endian PCM at
// sampleRate, frameSize samples per frame. Encoded access units are passed to
// onFrame as they become available. Talk-back is offered only when it is set.
var NewELDEncoder func(sampleRate, frameSize int, onFrame func(au []byte)) (io.WriteCloser, error)

const typeSpeaker = "113"

// talkbackMedia returns the backchannel media for accessories with a speaker
// that use AAC-ELD, or nil if talk-back isn't possible.
func talkbackMedia(acc *hap.Accessory, audio *camera.SupportedAudioStreamConfiguration) *core.Media {
	if NewELDEncoder == nil || acc.GetService(typeSpeaker) == nil {
		return nil
	}
	if len(audio.Codecs) == 0 || audio.Codecs[0].CodecType != camera.AudioCodecTypeAACELD {
		return nil
	}
	// The browser's microphone, in codecs every WebRTC client supports. The
	// samples are converted to the camera's rate before encoding.
	return &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000},
			{Name: core.CodecPCMU, ClockRate: 8000},
		},
	}
}

// talkback encodes backchannel audio to AAC-ELD and sends it to the camera.
type talkback struct {
	encoder io.WriteCloser
	pending []byte // PCM waiting for a complete frame
	frame   int    // bytes per frame
	rate    int    // samples per second

	mu  sync.Mutex
	seq uint16
	ts  uint32
}

func newTalkback(sampleRate, frameSize int, write func(*rtp.Packet)) (*talkback, error) {
	t := &talkback{frame: frameSize * 2, rate: sampleRate}

	var err error
	t.encoder, err = NewELDEncoder(sampleRate, frameSize, func(au []byte) {
		t.mu.Lock()
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				SequenceNumber: t.seq,
				Timestamp:      t.ts,
			},
			Payload: eldPayload(au),
		}
		t.seq++
		t.ts += uint32(frameSize)
		t.mu.Unlock()

		write(packet)
	})
	if err != nil {
		return nil, err
	}

	return t, nil
}

// WritePCM buffers 16-bit little-endian samples and passes whole frames on.
func (t *talkback) WritePCM(b []byte) error {
	t.pending = append(t.pending, b...)
	for len(t.pending) >= t.frame {
		if _, err := t.encoder.Write(t.pending[:t.frame]); err != nil {
			return err
		}
		t.pending = t.pending[t.frame:]
	}
	return nil
}

func (t *talkback) Close() error {
	return t.encoder.Close()
}

// eldPayload packs one access unit as RFC 3640 AAC-hbr, the same format the
// camera sends: a 16-bit AU-headers length, then one AU-header with a 13-bit
// size and 3-bit index.
func eldPayload(au []byte) []byte {
	payload := make([]byte, 4+len(au))
	binary.BigEndian.PutUint16(payload, 16)
	binary.BigEndian.PutUint16(payload[2:], uint16(len(au))<<3)
	copy(payload[4:], au)
	return payload
}

// AddTrack receives the backchannel (e.g. a browser's microphone) and sends it
// to the camera's speaker over the stream's audio session.
func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if c.talkback == nil {
		rate := int(audioSampleRates[c.audioConfig.Codecs[0].CodecParams[0].SampleRate[0]])
		// HomeKit's AAC-ELD frames are 480 samples, see camera.RTPTimeAACELD16
		tb, err := newTalkback(rate, 480, func(packet *rtp.Packet) {
			session := c.talkbackSession.Load()
			if session == nil {
				return // stream not started yet
			}
			if n, err := session.WriteRTP(packet); err == nil {
				c.Send += n
			}
		})
		if err != nil {
			return err
		}
		c.talkback = tb
	}

	dst := &core.Codec{Name: core.CodecPCML, ClockRate: uint32(c.talkback.rate), Channels: 1}
	transcode := pcm.Transcode(dst, track.Codec)

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		_ = c.talkback.WritePCM(transcode(packet.Payload))
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}
