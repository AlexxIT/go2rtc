package onvif

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestMotorSteps(t *testing.T) {
	cases := []struct {
		name     string
		x, y     float64
		ops      []int
		interval time.Duration
	}{
		{"frigate left", -0.5, 0, []int{opLeft}, 400 * time.Millisecond},
		{"frigate right", 0.5, 0, []int{opRight}, 400 * time.Millisecond},
		{"frigate up", 0, 0.5, []int{opUp}, 400 * time.Millisecond},
		{"frigate down", 0, -0.5, []int{opDown}, 400 * time.Millisecond},
		{"full speed is capped", 1, 0, []int{opRight}, 400 * time.Millisecond},
		{"slow push", 0.25, 0, []int{opRight}, 800 * time.Millisecond},
		{"very slow is capped", 0.12, 0, []int{opRight}, 1200 * time.Millisecond},
		{"diagonal", -0.5, 0.5, []int{opLeft, opUp}, 400 * time.Millisecond},
		{"drift", 0.05, -0.05, nil, 0},
		{"zero", 0, 0, nil, 0},
	}
	for _, c := range cases {
		ops, interval := motorSteps(c.x, c.y)
		if !slices.Equal(ops, c.ops) || interval != c.interval {
			t.Errorf("%s: motorSteps(%v, %v) = %v, %v; want %v, %v", c.name, c.x, c.y, ops, interval, c.ops, c.interval)
		}
	}
}

func TestParseVelocity(t *testing.T) {
	// As zeep (Frigate's ONVIF client) serialises a ContinuousMove.
	body := []byte(`<soap-env:Body><ns0:ContinuousMove xmlns:ns0="http://www.onvif.org/ver20/ptz/wsdl">` +
		`<ns0:ProfileToken>mi360</ns0:ProfileToken><ns0:Velocity>` +
		`<ns1:PanTilt xmlns:ns1="http://www.onvif.org/ver10/schema" x="-0.5" y="0"/>` +
		`</ns0:Velocity></ns0:ContinuousMove></soap-env:Body>`)
	if x, y := parseVelocity(body); x != -0.5 || y != 0 {
		t.Errorf("parseVelocity = %v, %v; want -0.5, 0", x, y)
	}

	// Attribute order is not significant in XML: y before x must parse the same.
	body = []byte(`<tptz:ContinuousMove><tptz:ProfileToken>mi360</tptz:ProfileToken>` +
		`<tptz:Velocity><tt:PanTilt y="0.5" x="-0.25" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocityGenericSpace"/>` +
		`</tptz:Velocity></tptz:ContinuousMove>`)
	if x, y := parseVelocity(body); x != -0.25 || y != 0.5 {
		t.Errorf("parseVelocity (y first) = %v, %v; want -0.25, 0.5", x, y)
	}

	// Single-quoted attributes are valid XML too.
	body = []byte(`<tt:PanTilt x='1' y='-1'/>`)
	if x, y := parseVelocity(body); x != 1 || y != -1 {
		t.Errorf("parseVelocity (single quotes) = %v, %v; want 1, -1", x, y)
	}

	// A Zoom element must not be mistaken for PanTilt.
	body = []byte(`<tt:Zoom x="0.7"/>`)
	if x, y := parseVelocity(body); x != 0 || y != 0 {
		t.Errorf("parseVelocity (zoom only) = %v, %v; want 0, 0", x, y)
	}
}

func TestEndMoveKeepsReplacement(t *testing.T) {
	const name = "test-end-move"
	defer stopMove(name)

	_, cancel1 := context.WithCancel(context.Background())
	old := &move{cancel: cancel1}
	movesMu.Lock()
	moves[name] = old
	movesMu.Unlock()

	// A second ContinuousMove replaces the first one.
	_, cancel2 := context.WithCancel(context.Background())
	stopMove(name)
	current := &move{cancel: cancel2}
	movesMu.Lock()
	moves[name] = current
	movesMu.Unlock()

	// The first goroutine finishes late: it must not remove its replacement.
	endMove(name, old)
	if !isMoving(name) {
		t.Fatal("stale endMove removed the replacement move")
	}

	// The owner does remove its own entry.
	endMove(name, current)
	if isMoving(name) {
		t.Fatal("endMove did not remove its own move")
	}
}
