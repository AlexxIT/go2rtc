package mp4

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/av1"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Connection
	wr    *core.WriteBuffer
	muxer *Muxer
	mu    sync.Mutex
	start bool

	// AV1 carries its codec params in the sequence header, so the init
	// segment can only be built once every AV1 track has seen one. pending
	// counts them, initCh is closed when the last one arrives.
	waitInit bool
	pending  int
	initCh   chan struct{}
	initOnce sync.Once
	stopCh   chan struct{}
	stopOnce sync.Once

	// OnInit is called from WriteTo after the init segment is generated,
	// just before writing data. Use this to send the correct content-type
	// to consumers (e.g. MSE) with actual codec parameters.
	OnInit func(contentType string) `json:"-"`

	Rotate int `json:"-"`
	ScaleX int `json:"-"`
	ScaleY int `json:"-"`
}

func NewConsumer(medias []*core.Media) *Consumer {
	if medias == nil {
		// default local medias
		medias = []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecH264},
					{Name: core.CodecH265},
					{Name: core.CodecAV1},
				},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecAAC},
				},
			},
		}
	}

	wr := core.NewWriteBuffer(nil)
	return &Consumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "mp4",
			Medias:     medias,
			Transport:  wr,
		},
		muxer:  &Muxer{},
		wr:     wr,
		initCh: make(chan struct{}),
		stopCh: make(chan struct{}),
	}
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := byte(len(c.Senders))

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				if !h264.IsKeyframe(packet.Payload) {
					return
				}
				c.start = true
			}

			// important to use Mutex because right fragment order
			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecH265:
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				if !h265.IsKeyframe(packet.Payload) {
					return
				}
				c.start = true
			}

			// important to use Mutex because right fragment order
			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h265.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h265.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecAV1:
		// AV1 has no codec params in the SDP. They come with the sequence
		// header, which encoders repeat in front of every keyframe, so the
		// init segment (av1C box) can only be built once one has arrived.
		seqHdrOK := av1.GetSequenceHeader(codec.FmtpLine) != nil
		if !seqHdrOK {
			// under Mutex because an earlier track's handler may already be
			// decrementing pending from its own goroutine
			c.mu.Lock()
			c.waitInit = true
			c.pending++
			c.mu.Unlock()
		}

		// own flag, because another video track may set c.start first
		var started bool

		handler.Handler = func(packet *rtp.Packet) {
			if !started {
				if !seqHdrOK {
					if seqHdr := av1.SequenceHeader(packet.Payload); seqHdr != nil {
						// under Mutex because Codecs reads it from other goroutines
						c.mu.Lock()
						codec.FmtpLine = av1.EncodeFmtpLine(seqHdr)
						c.pending--
						last := c.pending == 0
						c.mu.Unlock()

						seqHdrOK = true
						if last {
							c.initOnce.Do(func() { close(c.initCh) })
						}
					}
				}
				if !seqHdrOK || !av1.IsKeyframe(packet.Payload) {
					return
				}
				started = true
				c.start = true
			}

			// important to use Mutex because right fragment order
			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = av1.RTPDepay(handler.Handler)
		}

	default:
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				return
			}

			// important to use Mutex because right fragment order
			c.mu.Lock()
			b := c.muxer.GetPayload(trackID, packet)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		switch track.Codec.Name {
		case core.CodecAAC:
			if track.Codec.IsRTP() {
				handler.Handler = aac.RTPDepay(handler.Handler)
			}
		case core.CodecOpus, core.CodecMP3: // no changes
		case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
			codec.Name = core.CodecFLAC
			if codec.Channels == 2 {
				// hacky way for support two channels audio
				codec.Channels = 1
				codec.ClockRate *= 2
			}
			handler.Handler = pcm.FLACEncoder(track.Codec.Name, codec.ClockRate, handler.Handler)

		default:
			handler.Handler = nil
		}
	}

	if handler.Handler == nil {
		s := "mp4: unsupported codec: " + track.Codec.String()
		println(s)
		return errors.New(s)
	}

	c.muxer.AddTrack(codec)

	handler.HandleRTP(track)
	c.Senders = append(c.Senders, handler)

	return nil
}

// WaitInit blocks until the codec parameters for the init segment are known,
// the consumer is stopped, or the timeout expires. A zero timeout waits
// indefinitely. Returns false if the parameters are still unknown.
func (c *Consumer) WaitInit(timeout time.Duration) bool {
	if !c.waitInit {
		return true
	}

	var expired <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		expired = timer.C
	}

	select {
	case <-c.initCh:
		// select picks at random when both are ready, and continuing after a
		// Stop parks WriteTo in a write buffer nothing will release
		select {
		case <-c.stopCh:
			return false
		default:
			return true
		}
	case <-c.stopCh:
	case <-expired:
	}

	return false
}

func (c *Consumer) Stop() error {
	c.stopOnce.Do(func() { close(c.stopCh) })
	return c.Connection.Stop()
}

// Codecs returns a snapshot, because an AV1 track fills in its sequence header
// from a track handler while the stream is already running.
func (c *Consumer) Codecs() []*core.Codec {
	c.mu.Lock()
	defer c.mu.Unlock()

	codecs := c.Connection.Codecs()
	for i, codec := range codecs {
		codecs[i] = codec.Clone()
	}
	return codecs
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	if len(c.Senders) == 1 && c.Senders[0].Codec.IsAudio() {
		c.start = true
	}

	// AV1 has no codec params in the SDP, they come with the first keyframe
	if !c.WaitInit(0) {
		return 0, nil
	}

	init, err := c.muxer.GetInit()
	if err != nil {
		return 0, err
	}

	if c.Rotate != 0 {
		PatchVideoRotate(init, c.Rotate)
	}
	if c.ScaleX != 0 && c.ScaleY != 0 {
		PatchVideoScale(init, c.ScaleX, c.ScaleY)
	}

	// the caller can only know the final content type now
	if c.OnInit != nil {
		c.OnInit(ContentType(c.Codecs()))
	}

	if _, err = wr.Write(init); err != nil {
		return 0, err
	}

	return c.wr.WriteTo(wr)
}
