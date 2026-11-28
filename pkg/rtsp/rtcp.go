package rtsp

import (
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RFC 3550 requires RTP senders to send periodic RTCP Sender Reports.
// The NTP<->RTP mapping in them is the only way for a receiver to sync
// the clocks of multiple tracks (lipsync) and to map a stream onto its
// own wallclock. Some receivers (ex. UniFi Protect) log clock warnings,
// lose A/V sync or drift the recording timeline when SRs are missing.
// https://github.com/AlexxIT/go2rtc/issues/2303
const (
	srInterval  = 2500 * time.Millisecond // media senders commonly use 2..5s
	srMaxDrift  = 30 * time.Second        // hard re-anchor beyond this
	srSlewBand  = 150 * time.Millisecond  // ignore drift smaller than this
	srSlewStep  = 5 * time.Millisecond    // max timeline correction per report
	srStartBias = 100 * time.Millisecond  // assumed pipeline latency of the first frame
)

type senderReport struct {
	clockRate uint32
	packets   uint32
	octets    uint32
	anchorNTP time.Time // wallclock moment that maps to anchorTS
	anchorTS  uint32
	last      time.Time
}

func (s *senderReport) count(packet *rtp.Packet) {
	s.packets++
	s.octets += uint32(len(packet.Payload))
}

// marshal returns an interleaved framed compound RTCP packet (SR+SDES)
// for the given RTP timestamp when a report is due, otherwise nil.
//
// The NTP<->RTP mapping is anchored once and then advanced along the RTP
// timeline, slewed only a few ms per report toward wallclock. Stamping
// each report with the current wallclock instead would jitter the mapping
// on every delivery stall or burst, which receivers translate into stream
// clock jumps (UniFi Protect logs "streamClock regression" / "Back time"
// DTS corrections on every such report).
func (s *senderReport) marshal(channel uint8, ssrc, ts uint32, now time.Time) []byte {
	if now.Sub(s.last) < srInterval {
		return nil
	}
	s.last = now

	if s.anchorNTP.IsZero() {
		// the first frame was produced slightly in the past, and the small
		// bias makes later slew corrections mostly forward, which receivers
		// tolerate better than backward corrections
		s.anchorNTP = now.Add(-srStartBias)
	} else {
		// int32 diff handles timestamp wraparound and reordering
		diff := int32(ts - s.anchorTS)
		s.anchorNTP = s.anchorNTP.Add(time.Duration(diff) * time.Second / time.Duration(s.clockRate))

		// absorb long-term clock drift between producer and wallclock
		if drift := now.Sub(s.anchorNTP); drift > srMaxDrift || drift < -srMaxDrift {
			s.anchorNTP = now.Add(-srStartBias)
		} else if drift > srSlewBand {
			s.anchorNTP = s.anchorNTP.Add(srSlewStep)
		} else if drift < -srSlewBand {
			s.anchorNTP = s.anchorNTP.Add(-srSlewStep)
		}
	}
	s.anchorTS = ts

	sr := rtcp.SenderReport{
		SSRC:        ssrc,
		NTPTime:     ntpTime(s.anchorNTP),
		RTPTime:     ts,
		PacketCount: s.packets,
		OctetCount:  s.octets,
	}
	sd := rtcp.SourceDescription{
		Chunks: []rtcp.SourceDescriptionChunk{{
			Source: ssrc,
			Items:  []rtcp.SourceDescriptionItem{{Type: rtcp.SDESCNAME, Text: "go2rtc"}},
		}},
	}

	data, err := rtcp.Marshal([]rtcp.Packet{&sr, &sd})
	if err != nil {
		return nil
	}

	b := make([]byte, 4, 4+len(data))
	b[0] = '$'
	b[1] = channel
	b[2] = byte(len(data) >> 8)
	b[3] = byte(len(data))
	return append(b, data...)
}

// ntpTime converts wallclock time to a 64-bit fixed point NTP timestamp
func ntpTime(t time.Time) uint64 {
	secs := uint64(t.Unix()) + 2208988800 // seconds between 1900 and 1970
	frac := (uint64(t.Nanosecond()) << 32) / 1e9
	return secs<<32 | frac
}
