package rtsp

import (
	"net"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func parseSR(t *testing.T, b []byte, channel uint8) *rtcp.SenderReport {
	t.Helper()

	require.NotNil(t, b)
	require.Equal(t, byte('$'), b[0])
	require.Equal(t, channel, b[1])
	require.Equal(t, len(b)-4, int(b[2])<<8|int(b[3]))

	packets, err := rtcp.Unmarshal(b[4:])
	require.NoError(t, err)
	require.Len(t, packets, 2)

	sd, ok := packets[1].(*rtcp.SourceDescription)
	require.True(t, ok)
	require.Equal(t, rtcp.SDESCNAME, sd.Chunks[0].Items[0].Type)

	sr, ok := packets[0].(*rtcp.SenderReport)
	require.True(t, ok)
	return sr
}

func ntpToTime(ntp uint64) time.Time {
	secs := int64(ntp>>32) - 2208988800
	frac := int64((ntp & 0xFFFFFFFF) * 1e9 >> 32)
	return time.Unix(secs, frac)
}

func TestSenderReportBasic(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := senderReport{clockRate: 90000}

	s.count(&rtp.Packet{Payload: make([]byte, 100)})
	s.count(&rtp.Packet{Payload: make([]byte, 50)})

	sr := parseSR(t, s.marshal(3, 0x11223344, 1000, now), 3)
	require.Equal(t, uint32(0x11223344), sr.SSRC)
	require.Equal(t, uint32(1000), sr.RTPTime)
	require.Equal(t, uint32(2), sr.PacketCount)
	require.Equal(t, uint32(150), sr.OctetCount)

	// first report maps the RTP timestamp slightly into the past
	require.WithinDuration(t, now.Add(-srStartBias), ntpToTime(sr.NTPTime), time.Millisecond)
}

func TestSenderReportInterval(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := senderReport{clockRate: 90000}

	require.NotNil(t, s.marshal(1, 1, 0, now))
	require.Nil(t, s.marshal(1, 1, 3000, now.Add(time.Second)))
	require.NotNil(t, s.marshal(1, 1, 90000*3, now.Add(3*time.Second)))
}

func TestSenderReportMappingConsistency(t *testing.T) {
	// the NTP<->RTP mapping must advance along the RTP timeline even when
	// wallclock delivery is bursty, otherwise receivers see clock jumps
	const clockRate = 90000
	base := time.Unix(1700000000, 0)
	s := senderReport{clockRate: clockRate}

	first := parseSR(t, s.marshal(1, 1, 0, base), 1)

	for i := 1; i <= 10; i++ {
		ts := uint32(i) * 3 * clockRate // 3s of media per report
		mediaTime := time.Duration(i) * 3 * time.Second
		// delivery jitter oscillates around zero and stays inside the
		// slew dead band, so it must never disturb the mapping
		jitter := time.Duration(1-2*(i%2)) * 40 * time.Millisecond
		now := base.Add(mediaTime + jitter)

		sr := parseSR(t, s.marshal(1, 1, ts, now), 1)

		ntpDelta := ntpToTime(sr.NTPTime).Sub(ntpToTime(first.NTPTime))
		require.InDelta(t, mediaTime, ntpDelta, float64(time.Millisecond),
			"report %d: NTP advance must equal RTP advance", i)
	}
}

func TestSenderReportSlewBounds(t *testing.T) {
	const clockRate = 8000
	now := time.Unix(1700000000, 0)
	s := senderReport{clockRate: clockRate}

	prev := parseSR(t, s.marshal(1, 1, 0, now), 1)

	// producer runs 1% slow vs wallclock: drift accumulates, correction
	// must stay bounded to srSlewStep per report
	ts := uint32(0)
	for i := 1; i <= 5; i++ {
		ts += 3 * clockRate
		now = now.Add(3*time.Second + 500*time.Millisecond) // way past dead band

		sr := parseSR(t, s.marshal(1, 1, ts, now), 1)

		mediaTime := 3 * time.Second
		ntpDelta := ntpToTime(sr.NTPTime).Sub(ntpToTime(prev.NTPTime))
		require.LessOrEqual(t, (ntpDelta - mediaTime).Abs(), srSlewStep+time.Millisecond,
			"report %d: correction exceeds slew step", i)
		prev = sr
	}
}

func TestSenderReportHardReset(t *testing.T) {
	const clockRate = 90000
	now := time.Unix(1700000000, 0)
	s := senderReport{clockRate: clockRate}

	parseSR(t, s.marshal(1, 1, 0, now), 1)

	// producer stalled for a minute with no RTP advance: drift exceeds
	// srMaxDrift, mapping must re-anchor to wallclock
	now = now.Add(time.Minute)
	sr := parseSR(t, s.marshal(1, 1, 3000, now), 1)
	require.WithinDuration(t, now.Add(-srStartBias), ntpToTime(sr.NTPTime), time.Millisecond)
}

func TestSenderReportTimestampWraparound(t *testing.T) {
	const clockRate = 90000
	now := time.Unix(1700000000, 0)
	s := senderReport{clockRate: clockRate}

	start := uint32(0xFFFFFFFF - clockRate) // 1s before wrap
	first := parseSR(t, s.marshal(1, 1, start, now), 1)

	now = now.Add(3 * time.Second)
	sr := parseSR(t, s.marshal(1, 1, start+3*clockRate, now), 1) // wrapped

	ntpDelta := ntpToTime(sr.NTPTime).Sub(ntpToTime(first.NTPTime))
	require.InDelta(t, 3*time.Second, ntpDelta, float64(time.Millisecond))
}

func TestNTPTime(t *testing.T) {
	// known value: 1900-01-01 maps to 0
	require.Equal(t, uint64(0), ntpTime(time.Unix(-2208988800, 0)))
	// half second fraction
	require.Equal(t, uint64(2208988800)<<32|1<<31, ntpTime(time.Unix(0, 5e8)))
}

func TestSenderReportUDPRouting(t *testing.T) {
	// for transport=udp the framed report must arrive unframed on the
	// RTCP socket of the pair (odd channel)
	rtpConn, rtcpConn, err := ListenUDPPair()
	require.NoError(t, err)
	defer rtpConn.Close()
	defer rtcpConn.Close()

	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer sink.Close()

	c := &Conn{
		Transport: "udp",
		udpConn:   []*net.UDPConn{rtpConn, rtcpConn},
		udpAddr: []*net.UDPAddr{
			sink.LocalAddr().(*net.UDPAddr), // RTP (unused here)
			sink.LocalAddr().(*net.UDPAddr), // RTCP
		},
	}

	s := senderReport{clockRate: 90000}
	b := s.marshal(1, 0x42, 90000, time.Unix(1700000000, 0))
	require.NoError(t, c.writeInterleavedData(b))

	_ = sink.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1500)
	n, _, err := sink.ReadFromUDP(buf)
	require.NoError(t, err)

	packets, err := rtcp.Unmarshal(buf[:n])
	require.NoError(t, err)
	sr, ok := packets[0].(*rtcp.SenderReport)
	require.True(t, ok)
	require.Equal(t, uint32(0x42), sr.SSRC)
}
