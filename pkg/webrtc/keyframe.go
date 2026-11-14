package webrtc

import (
	"errors"
	"io"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// maxRTCPReadErrors ends an RTCP read loop after this many consecutive failed
// reads without a successful one in between. A single unparsable packet from
// a viewer must not stop NACK and PLI handling; a reader that only fails must
// not spin forever. A viewer that sends only garbage ends the loop of its own
// senders only; the loops of other viewers are not affected.
const maxRTCPReadErrors = 64

var _ core.KeyframeRequestSource = (*Conn)(nil)

// OnKeyframeRequest registers handler for RTCP PLI/FIR feedback that viewers
// send for the sender of media (matched by media.ID, the MID). A later call
// for the same media replaces the handler, a nil handler removes it.
//
// The handler runs in the RTCP read loop of that sender and must return
// immediately. Blocking delays NACK handling for the viewer.
func (c *Conn) OnKeyframeRequest(media *core.Media, handler func()) {
	c.keyframeMu.Lock()
	defer c.keyframeMu.Unlock()
	if handler == nil {
		delete(c.keyframeHandlers, media.ID)
		return
	}
	if c.keyframeHandlers == nil {
		c.keyframeHandlers = map[string]func(){}
	}
	c.keyframeHandlers[media.ID] = handler
}

// keyframeHandler returns the handler for mid, or nil.
func (c *Conn) keyframeHandler(mid string) func() {
	c.keyframeMu.Lock()
	defer c.keyframeMu.Unlock()
	return c.keyframeHandlers[mid]
}

// senderMid returns the MID of the transceiver that owns sender, or "".
func (c *Conn) senderMid(sender *webrtc.RTPSender) string {
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Sender() == sender {
			return tr.Mid()
		}
	}
	return ""
}

// readSenderRTCP keeps pion's interceptors (NACK limiter and responder) running
// for sender and reports PLI/FIR to the handler registered for its MID.
func (c *Conn) readSenderRTCP(sender *webrtc.RTPSender) {
	readMidRTCP(c.senderMid(sender), sender.Read, c.keyframeHandler)
}

// readMidRTCP runs readRTCP for the sender with MID mid and logs at debug
// level when the loop gives up on its error budget. The log carries only the
// MID and the count: the read error may contain the viewer's address.
func readMidRTCP(mid string, read func(b []byte) (int, interceptor.Attributes, error), handlerFor func(mid string) func()) {
	gaveUp := readRTCP(read, func() {
		if handler := handlerFor(mid); handler != nil {
			handler()
		}
	})
	if gaveUp {
		log.Debug().Str("mid", mid).
			Msgf("[webrtc] RTCP read loop stopped after %d consecutive errors", maxRTCPReadErrors)
	}
}

// wantsKeyframe reports whether pkts contain a keyframe request (PLI or FIR).
func wantsKeyframe(pkts []rtcp.Packet) bool {
	for _, pkt := range pkts {
		switch pkt.(type) {
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			return true
		}
	}
	return false
}

// rtcpReadDone reports whether err means the RTCP reader is closed for good.
// pion returns io.ErrClosedPipe after the sender stopped (rtpsender.go Read)
// and io.EOF or net.ErrClosed when the SRTCP stream is closed.
func rtcpReadDone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed)
}

// readRTCP reads RTCP until the reader is closed (rtcpReadDone) or after
// maxRTCPReadErrors consecutive errors. Packets are taken from the attribute
// cache the interceptors share, so a PLI next to a NACK the limiter emptied is
// seen even if the raw bytes could not be rebuilt. onKeyframe is called once
// per read that contains PLI or FIR; it runs in this loop and must return
// immediately. It reports whether it stopped because of the error budget.
//
// sender.ReadRTCP is not used: it parses the raw bytes again and fails on the
// whole compound if a single packet in it is invalid.
func readRTCP(read func(b []byte) (int, interceptor.Attributes, error), onKeyframe func()) (gaveUp bool) {
	buf := make([]byte, 1500)
	errs := 0
	for {
		n, attr, err := read(buf)
		if err != nil {
			if rtcpReadDone(err) {
				return false
			}
			if errs++; errs >= maxRTCPReadErrors {
				return true
			}
			continue
		}
		errs = 0

		var pkts []rtcp.Packet
		if attr != nil {
			pkts, err = attr.GetRTCPPackets(buf[:n])
		} else {
			pkts, err = rtcp.Unmarshal(buf[:n])
		}
		if err != nil {
			continue
		}
		if wantsKeyframe(pkts) {
			onKeyframe()
		}
	}
}
