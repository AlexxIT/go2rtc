package mpegts

// Clock converts RTP timestamps of any clock rate to the 90 kHz clock of PTS.
//
// Only the difference between timestamps is scaled, not the absolute value.
// RTP timestamps start at a random uint32, and scaling a large absolute value
// by 90000/rate would not fit in uint32 (the float to uint32 conversion is
// implementation-defined in that case). The uint32 subtraction also handles
// the RTP timestamp wraparound. The result is not related to the source
// timestamp; the muxer only uses differences between PTS values.
type Clock struct {
	rate  uint64
	last  uint32
	total uint64 // source ticks since the first timestamp
	init  bool
}

func NewClock(rate uint32) *Clock {
	if rate == 0 {
		rate = ClockRate
	}
	return &Clock{rate: uint64(rate)}
}

// Convert returns the 90 kHz timestamp for the next RTP timestamp.
func (c *Clock) Convert(ts uint32) uint32 {
	if c.init {
		c.total += uint64(ts - c.last)
	}
	c.init = true
	c.last = ts
	// scale the total, not each step, so rounding doesn't add up over time;
	// +1 because Muxer.GetPayload treats zero as "no previous timestamp"
	return uint32(c.total*ClockRate/c.rate) + 1
}
