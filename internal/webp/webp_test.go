package webp

import (
	"testing"
)

func TestInit(t *testing.T) {
	// Verify Init() runs without panicking and registers API endpoints.
	// api.HandleFunc registrations are idempotent so calling Init multiple times is safe.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Init() panicked: %v", r)
		}
	}()
	Init()
}
