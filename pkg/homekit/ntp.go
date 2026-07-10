package homekit

import "time"

// ntpEpochOffset is seconds between NTP epoch (1900) and Unix epoch (1970)
const ntpEpochOffset = 2208988800

// TimeToNTP converts a wall clock time to a 64-bit NTP timestamp
func TimeToNTP(t time.Time) uint64 {
	if t.IsZero() {
		t = time.Now()
	}
	secs := uint64(t.Unix()) + ntpEpochOffset
	// fractional second in 1/2^32 units
	frac := uint64(uint32(uint64(t.Nanosecond()) * 0x100000000 / 1e9))
	return secs<<32 | frac
}

// NTPToTime converts a 64-bit NTP timestamp to wall clock time
func NTPToTime(ntp uint64) time.Time {
	if ntp == 0 {
		return time.Time{}
	}
	secs := int64(ntp>>32) - ntpEpochOffset
	frac := ntp & 0xffffffff
	nsec := int64(frac * 1e9 / 0x100000000)
	return time.Unix(secs, nsec)
}
