package streams

import (
	"testing"
	"time"
)

func TestPreloadRetryDelayBacksOffAndCaps(t *testing.T) {
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i, d := range want {
		if got := preloadRetryDelay(i); got != d {
			t.Fatalf("attempt %d: want %s, got %s", i, d, got)
		}
	}
}
