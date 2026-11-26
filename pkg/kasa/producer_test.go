package kasa

import (
	"net/http"
	"testing"
	"time"
)

func TestGetTimestampKeepsFramesApart(t *testing.T) {
	at := func(stamp string) float64 {
		h := http.Header{}
		h.Set("X-Timestamp", stamp)
		return GetTimestamp(h)
	}

	if d := at("3000000.133") - at("3000000.066"); d < 0.066 || d > 0.068 {
		t.Fatalf("frames stamped 67 ms apart came out %.3f s apart", d)
	}
}

func TestGetTimestampFallbackKeepsSubseconds(t *testing.T) {
	before := time.Now()
	ts := GetTimestamp(make(http.Header))
	after := time.Now()

	if got := time.Unix(0, int64(ts*float64(time.Second))); got.Before(before.Add(-time.Microsecond)) || got.After(after.Add(time.Microsecond)) {
		t.Fatalf("fallback timestamp %v is outside [%v, %v]", got, before, after)
	}
}
