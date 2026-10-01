package mp4

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/av1"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Keyframe struct {
	core.Connection
	wr    *core.WriteBuffer
	muxer *Muxer
}

// Deprecated: should be rewritten
func NewKeyframe(medias []*core.Media) *Keyframe {
	if medias == nil {
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
		}
	}

	wr := core.NewWriteBuffer(nil)
	cons := &Keyframe{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "mp4",
			Transport:  wr,
		},
		muxer: &Muxer{},
		wr:    wr,
	}
	cons.Medias = medias
	return cons
}

func (c *Keyframe) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	// clone because the AV1 branch fills FmtpLine from the first keyframe
	codec := track.Codec.Clone()
	c.muxer.AddTrack(codec)

	// AV1 has no codec params in the SDP, its init segment is built below
	var init []byte
	if track.Codec.Name != core.CodecAV1 {
		var err error
		if init, err = c.muxer.GetInit(); err != nil {
			return err
		}
	}

	// the clone, so Codecs reports the params the AV1 branch fills in below
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		handler.Handler = func(packet *rtp.Packet) {
			if !h264.IsKeyframe(packet.Payload) {
				return
			}

			// important to use Mutex because right fragment order
			b := c.muxer.GetPayload(0, packet)
			b = append(init, b...)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecH265:
		handler.Handler = func(packet *rtp.Packet) {
			if !h265.IsKeyframe(packet.Payload) {
				return
			}

			// important to use Mutex because right fragment order
			b := c.muxer.GetPayload(0, packet)
			b = append(init, b...)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			handler.Handler = h265.RTPDepay(track.Codec, handler.Handler)
		}

	case core.CodecAV1:
		seqHdrOK := av1.GetSequenceHeader(codec.FmtpLine) != nil

		handler.Handler = func(packet *rtp.Packet) {
			// the av1C box needs the sequence header, which encoders repeat in
			// front of every keyframe
			if !seqHdrOK {
				seqHdr := av1.SequenceHeader(packet.Payload)
				if seqHdr == nil {
					return
				}
				codec.FmtpLine = av1.EncodeFmtpLine(seqHdr)
				seqHdrOK = true
			}

			if !av1.IsKeyframe(packet.Payload) {
				return
			}

			init, err := c.muxer.GetInit()
			if err != nil {
				return
			}

			b := append(init, c.muxer.GetPayload(0, packet)...)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			handler.Handler = av1.RTPDepay(handler.Handler)
		}
	}

	handler.HandleRTP(track)
	c.Senders = append(c.Senders, handler)

	return nil
}

func (c *Keyframe) WriteTo(wr io.Writer) (int64, error) {
	return c.wr.WriteTo(wr)
}
