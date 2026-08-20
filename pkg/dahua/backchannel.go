package dahua

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// Backchannel is a go2rtc consumer that forwards an audio track to the
// camera speaker over the NetSDKTransport.
type Backchannel struct {
	core.Connection

	transport *NetSDKTransport

	mu      sync.Mutex
	buf     []byte
	closed  bool
	stopped chan struct{}
}

func (c *Backchannel) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

// Start blocks until the connection is torn down.
func (c *Backchannel) Start() error {
	<-c.stopped
	return nil
}

func (c *Backchannel) Stop() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.stopped)
		// Flush any partially-filled re-chunk buffer so the tail of the
		// conversation is not silently dropped (at most one frame period).
		if c.transport != nil && len(c.buf) > 0 {
			if n, err := c.transport.WriteAudio(c.buf); err == nil {
				c.Send += n
			}
			c.buf = nil
		}
	}
	c.mu.Unlock()

	if c.transport != nil {
		_ = c.transport.Close()
	}

	return c.Connection.Stop()
}

func (c *Backchannel) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if codec == nil {
		codec = track.Codec
	}

	if err := c.transport.Open(codec); err != nil {
		return err
	}

	frameSize := c.transport.FrameSize()

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.closed {
			return
		}

		if frameSize <= 0 {
			if n, err := c.transport.WriteAudio(packet.Payload); err == nil {
				c.Send += n
			}
			return
		}

		// Re-chunk the RTP payload into fixed size frames. Dahua devices
		// produce audible clicks when frame boundaries move around.
		c.buf = append(c.buf, packet.Payload...)

		for len(c.buf) >= frameSize {
			n, err := c.transport.WriteAudio(c.buf[:frameSize])
			if err != nil {
				return
			}
			c.Send += n
			c.buf = c.buf[frameSize:]
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)

	return nil
}
