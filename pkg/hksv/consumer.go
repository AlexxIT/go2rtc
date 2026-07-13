// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

// HKSVConsumer implements core.Consumer, generates fMP4 and sends over HDS.
// It can be pre-started without an HDS session, buffering init data until activated.
type HKSVConsumer struct {
	core.Connection
	muxer *mp4.Muxer
	mu    sync.Mutex
	done  chan struct{}
	log   zerolog.Logger

	// Set by Activate() when HDS session is available
	session  *hds.Session
	streamID int
	seqNum   int
	active   bool
	start    bool // waiting for first keyframe

	// GOP buffer - accumulate moof+mdat pairs, flush on next keyframe
	fragBuf []byte

	// Pre-built init segment (built when tracks connect)
	initData []byte
	initErr  error
	initDone chan struct{} // closed when init is ready
}

// NewHKSVConsumer creates a new HKSV consumer that muxes H264+AAC into fMP4
// and sends fragments over an HDS DataStream session.
func NewHKSVConsumer(log zerolog.Logger) *HKSVConsumer {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
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
	return &HKSVConsumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "hksv",
			Protocol:   "hds",
			Medias:     medias,
		},
		muxer:    &mp4.Muxer{},
		done:     make(chan struct{}),
		initDone: make(chan struct{}),
		log:      log,
	}
}

func (c *HKSVConsumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	// Reject late tracks after init segment is built (can't modify fMP4 header)
	select {
	case <-c.initDone:
		c.log.Debug().Str("codec", track.Codec.Name).Msg("[hksv] ignoring late track (init already built)")
		return nil
	default:
	}

	trackID := byte(len(c.Senders))

	c.log.Debug().Str("codec", track.Codec.Name).Uint8("trackID", trackID).Msg("[hksv] AddTrack")

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		handler.Handler = func(packet *rtp.Packet) {
			c.mu.Lock()
			if !c.active {
				c.mu.Unlock()
				return
			}
			if !c.start {
				if !h264.IsKeyframe(packet.Payload) {
					c.mu.Unlock()
					return
				}
				c.start = true
				c.log.Debug().Int("payloadLen", len(packet.Payload)).Msg("[hksv] first keyframe")
			} else if h264.IsKeyframe(packet.Payload) && len(c.fragBuf) > 0 {
				// New keyframe = flush previous GOP as one mediaFragment
				c.flushFragment()
			}

			b := c.muxer.GetPayload(trackID, packet)
			c.fragBuf = append(c.fragBuf, b...)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecAAC:
		handler.Handler = func(packet *rtp.Packet) {
			c.mu.Lock()
			if !c.active || !c.start {
				c.mu.Unlock()
				return
			}

			b := c.muxer.GetPayload(trackID, packet)
			c.fragBuf = append(c.fragBuf, b...)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = aac.RTPDepay(handler.Handler)
		}

	default:
		return nil // skip unsupported codecs
	}

	c.muxer.AddTrack(codec)
	handler.HandleRTP(track)
	c.Senders = append(c.Senders, handler)

	// Build init segment when all expected tracks are ready
	select {
	case <-c.initDone:
		// already built — ignore late tracks (init is immutable)
	default:
		if len(c.Senders) >= len(c.Medias) {
			c.buildInit()
		}
	}

	return nil
}

// buildInit creates the init segment from currently connected tracks.
// Must only be called once (closes initDone).
func (c *HKSVConsumer) buildInit() {
	initData, err := c.muxer.GetInit()
	c.initData = initData
	c.initErr = err
	close(c.initDone)
	if err != nil {
		c.log.Error().Err(err).Msg("[hksv] GetInit failed")
	} else {
		c.log.Debug().Int("initSize", len(initData)).Int("tracks", len(c.Senders)).Msg("[hksv] init segment ready")
	}
}

// Activate is called when the HDS session is ready (dataSend.open).
// It sends the pre-built init segment and starts streaming.
func (c *HKSVConsumer) Activate(session *hds.Session, streamID int) error {
	// Wait for init to be ready (should already be done if consumer was pre-started)
	select {
	case <-c.initDone:
	case <-time.After(5 * time.Second):
		// Build init with whatever tracks we have (audio may be missing)
		select {
		case <-c.initDone:
		default:
			if len(c.Senders) > 0 {
				c.log.Warn().Int("tracks", len(c.Senders)).Msg("[hksv] init timeout, building with available tracks")
				c.buildInit()
			} else {
				return errors.New("hksv: no tracks connected after timeout")
			}
		}
	}

	if c.initErr != nil {
		return c.initErr
	}

	c.log.Debug().Int("initSize", len(c.initData)).Msg("[hksv] sending init segment")

	if err := session.SendMediaInit(streamID, c.initData); err != nil {
		return err
	}

	c.log.Debug().Msg("[hksv] init segment sent OK")

	// Enable live streaming (seqNum=2 because init used seqNum=1)
	c.mu.Lock()
	c.session = session
	c.streamID = streamID
	c.seqNum = 2
	c.active = true
	c.mu.Unlock()

	return nil
}

// flushFragment sends the accumulated GOP buffer as a single mediaFragment.
// Must be called while holding c.mu.
func (c *HKSVConsumer) flushFragment() {
	fragment := c.fragBuf
	c.fragBuf = make([]byte, 0, len(fragment))

	c.log.Debug().Int("fragSize", len(fragment)).Int("seq", c.seqNum).Msg("[hksv] flush fragment")

	if err := c.session.SendMediaFragment(c.streamID, fragment, c.seqNum); err == nil {
		c.Send += len(fragment)
	}
	c.seqNum++
}

func (c *HKSVConsumer) WriteTo(io.Writer) (int64, error) {
	<-c.done
	return 0, nil
}

func (c *HKSVConsumer) Stop() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.mu.Lock()
	c.active = false
	c.mu.Unlock()
	return c.Connection.Stop()
}

// Done returns a channel that is closed when the consumer is stopped.
func (c *HKSVConsumer) Done() <-chan struct{} {
	return c.done
}

func (c *HKSVConsumer) String() string {
	return "hksv consumer"
}
