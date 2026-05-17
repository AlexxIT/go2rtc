package baichuan

import "testing"

func TestParseMotionStateMatchingChannel(t *testing.T) {
	t.Parallel()

	xmlText := `<?xml version="1.0" encoding="utf-8"?><body><AlarmEventList><AlarmEvent><channelId>0</channelId><status>none</status><AItype>none</AItype></AlarmEvent></AlarmEventList></body>`

	motion, matched, err := parseMotionState(xmlText, 0)
	if err != nil {
		t.Fatalf("parseMotionState() error = %v", err)
	}
	if !matched {
		t.Fatalf("parseMotionState() matched = false, want true")
	}
	if motion {
		t.Fatalf("parseMotionState() motion = true, want false")
	}
}

func TestParseMotionStateIgnoresOtherChannels(t *testing.T) {
	t.Parallel()

	xmlText := `<?xml version="1.0" encoding="utf-8"?><body><AlarmEventList><AlarmEvent><channelId>1</channelId><status>MD</status><AItype>people</AItype></AlarmEvent></AlarmEventList></body>`

	motion, matched, err := parseMotionState(xmlText, 0)
	if err != nil {
		t.Fatalf("parseMotionState() error = %v", err)
	}
	if matched {
		t.Fatalf("parseMotionState() matched = true, want false")
	}
	if motion {
		t.Fatalf("parseMotionState() motion = true, want false")
	}
}

func TestParseAbilityInfoFiltersByChannel(t *testing.T) {
	t.Parallel()

	xmlText := `<?xml version="1.0" encoding="utf-8"?><body><AbilityInfo><userName>admin</userName><alarm><subModule><channelId>1</channelId><abilityValue>motion_rw</abilityValue></subModule><subModule><channelId>0</channelId><abilityValue>motion_ro,rfAlarm_ro</abilityValue></subModule></alarm><system><subModule><abilityValue>version_ro</abilityValue></subModule></system></AbilityInfo></body>`

	abilities, err := parseAbilityInfo(xmlText, 0)
	if err != nil {
		t.Fatalf("parseAbilityInfo() error = %v", err)
	}
	if got, want := abilities["motion"], abilityReadOnly; got != want {
		t.Fatalf("abilities[motion] = %v, want %v", got, want)
	}
	if got, want := abilities["rfalarm"], abilityReadOnly; got != want {
		t.Fatalf("abilities[rfalarm] = %v, want %v", got, want)
	}
	if got, want := abilities["version"], abilityReadOnly; got != want {
		t.Fatalf("abilities[version] = %v, want %v", got, want)
	}
}
