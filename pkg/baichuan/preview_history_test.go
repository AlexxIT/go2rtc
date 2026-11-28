package baichuan

import (
	"bytes"
	"context"
	"testing"
)

func TestPreviewHistoryRequest(t *testing.T) {
	live, err := buildPreview(0, StreamMain, 0, 0)
	if err != nil || bytes.Contains(live, []byte("preRec")) {
		t.Fatalf("live request changed: %s (%v)", live, err)
	}
	history, err := buildPreview(0, StreamMain, 0, 25)
	if err != nil || !bytes.Contains(history, []byte("<preRec>25</preRec>")) {
		t.Fatalf("history request missing preRec: %s (%v)", history, err)
	}
	if _, err = (*Client)(nil).StartPreviewHistory(context.Background(), 0, StreamMain, 26); err == nil {
		t.Fatal("out-of-range history request admitted")
	}
}
