package webrtc

import (
	"errors"
	"io"
	"net"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
)

// maxRTCPReadErrors ends the RTCP read loop after this many failed reads in a row.
const maxRTCPReadErrors = 64

// rtcpReadDone reports whether err means the RTCP reader is closed for good.
func rtcpReadDone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed)
}

// readRTCP reads RTCP from one sender until it is closed. Reading keeps Pion's
// interceptors (ex. the NACK responder) running. PLI/FIR ask the source of
// the track for a keyframe, if there is a track.
// The interceptors return a broken or too large packet as a read error: skip
// it, end only when the reader is closed or too many reads in a row failed.
func readRTCP(read func([]byte) (int, interceptor.Attributes, error), track *Track) {
	b := make([]byte, 1500)
	for errs := 0; ; {
		n, _, err := read(b)
		if err != nil {
			if errs++; rtcpReadDone(err) || errs >= maxRTCPReadErrors {
				return
			}
			continue
		}
		errs = 0

		if track == nil {
			continue
		}
		pkts, _ := rtcp.Unmarshal(b[:n])
		for _, pkt := range pkts {
			switch pkt.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				track.requestKeyframe()
			}
		}
	}
}
