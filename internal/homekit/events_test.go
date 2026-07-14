package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
)

func TestFindSwitchEventIID_DoorbellService(t *testing.T) {
	acc := &hap.Accessory{
		AID: 1,
		Services: []*hap.Service{
			{Type: "3E"}, // AccessoryInformation
			{
				Type: TypeDoorbellService, // "121"
				Characters: []*hap.Character{
					{Type: "73", IID: 900},
				},
			},
		},
	}

	iid := findSwitchEventIID(acc)
	if iid != 900 {
		t.Fatalf("expected IID 900, got %d", iid)
	}
}

func TestFindSwitchEventIID_StatelessSwitch(t *testing.T) {
	acc := &hap.Accessory{
		AID: 1,
		Services: []*hap.Service{
			{Type: "3E"},
			{
				Type: TypeStatelessProgrammableSwitch, // "89"
				Characters: []*hap.Character{
					{Type: "73", IID: 500},
				},
			},
		},
	}

	iid := findSwitchEventIID(acc)
	if iid != 500 {
		t.Fatalf("expected IID 500, got %d", iid)
	}
}

func TestFindSwitchEventIID_DoorbellPreferred(t *testing.T) {
	// If both Doorbell and StatelessProgrammableSwitch exist,
	// the Doorbell service should be preferred.
	acc := &hap.Accessory{
		AID: 1,
		Services: []*hap.Service{
			{
				Type: TypeStatelessProgrammableSwitch,
				Characters: []*hap.Character{
					{Type: "73", IID: 500},
				},
			},
			{
				Type: TypeDoorbellService,
				Characters: []*hap.Character{
					{Type: "73", IID: 900},
				},
			},
		},
	}

	iid := findSwitchEventIID(acc)
	if iid != 900 {
		t.Fatalf("expected IID 900 (doorbell preferred), got %d", iid)
	}
}

func TestFindSwitchEventIID_FallbackAnyService(t *testing.T) {
	acc := &hap.Accessory{
		AID: 1,
		Services: []*hap.Service{
			{
				Type: "FF", // unknown service
				Characters: []*hap.Character{
					{Type: "73", IID: 123},
				},
			},
		},
	}

	iid := findSwitchEventIID(acc)
	if iid != 123 {
		t.Fatalf("expected IID 123 (fallback), got %d", iid)
	}
}

func TestFindSwitchEventIID_NotFound(t *testing.T) {
	acc := &hap.Accessory{
		AID: 1,
		Services: []*hap.Service{
			{
				Type: "110", // CameraRTPStreamManagement
				Characters: []*hap.Character{
					{Type: "114", IID: 10},
				},
			},
		},
	}

	iid := findSwitchEventIID(acc)
	if iid != 0 {
		t.Fatalf("expected IID 0 (not found), got %d", iid)
	}
}

func TestSSEListeners(t *testing.T) {
	ch := make(chan DoorbellEvent, 8)
	addSSEListener(ch)

	ev := DoorbellEvent{
		Stream: "test",
		Event:  "single_press",
		Value:  0,
	}
	notifySSEListeners(ev)

	select {
	case got := <-ch:
		if got.Stream != "test" || got.Event != "single_press" {
			t.Fatalf("unexpected event: %+v", got)
		}
	default:
		t.Fatal("expected event on channel")
	}

	removeSSEListener(ch)
	notifySSEListeners(ev)

	select {
	case <-ch:
		t.Fatal("should not receive event after removal")
	default:
		// expected
	}
}
