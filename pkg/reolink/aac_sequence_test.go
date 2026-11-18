package reolink

import "testing"

func TestAACSequencePublication(t *testing.T) {
	for _, test := range []struct {
		name           string
		audio, emitted bool
		aac            bool
		want           aacSequence
	}{
		{"AAC", true, true, true, aacSequence{next: 0, config: 24, valid: true}},
		{"ADPCM", true, true, false, aacSequence{next: 0, config: 24}},
		{"no emission", true, false, true, aacSequence{next: 65535, config: 16, valid: true}},
		{"unemitted ADPCM", true, false, false, aacSequence{next: 65535, config: 16, valid: true}},
		{"audio muted", false, true, false, aacSequence{next: 65535, config: 16, valid: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := ProfileKey{}
			camera := &Camera{epochs: map[ProfileKey]mediaEpoch{key: {
				video: 90000, audio: 16000, audioRate: 16000, videoSet: true, audioSet: true,
				aac: aacSequence{next: 65535, config: 16, valid: true},
			}}}
			epoch := camera.nextEpoch(key, 180000, test.audio)
			if epoch.video != 180000 || camera.epochs[key].video != 90000 {
				t.Fatal("prospective Dial clock was persisted")
			}
			camera.recordEpoch(key, mediaEpoch{video: 1, audio: 1, audioRate: 16000,
				videoSet: true, audioSet: true, aac: aacSequence{config: 24, valid: test.aac}}, test.audio, test.emitted)
			got := camera.epochs[key]
			if got.aac != test.want || got.video != 90000 || got.audio != 16000 {
				t.Fatalf("header publication and clock maxima were coupled: %+v", got)
			}
		})
	}
}

func TestAACSequenceOverlap(t *testing.T) {
	for _, order := range [][2]int{{0, 1}, {1, 0}} {
		key := ProfileKey{}
		camera := &Camera{epochs: map[ProfileKey]mediaEpoch{key: {
			aac: aacSequence{next: 10, config: 16, valid: true},
		}}}
		first := camera.nextEpoch(key, 0, true)
		second := camera.nextEpoch(key, 0, true)
		if !first.aac.valid || second.aac.valid || camera.epochs[key].aac.valid {
			t.Fatal("overlap did not invalidate stored and returned carry")
		}
		values := [2]mediaEpoch{first, second}
		for i := range values {
			values[i].aac.next = uint16(20 + i)
			values[i].aac.valid = true
		}
		camera.recordEpoch(key, values[order[0]], true, true)
		third := camera.nextEpoch(key, 0, true)
		if third.aac.valid || !third.aac.shared || third.aac.users != 2 {
			t.Fatal("third Dial reused a partially departed shared cohort")
		}
		camera.recordEpoch(key, values[order[1]], true, true)
		camera.recordEpoch(key, first, true, true)
		if state := camera.epochs[key].aac; state.valid || state.users != 0 || !state.shared {
			t.Fatalf("last overlapping finisher published carry: %+v", state)
		}
		fresh := camera.nextEpoch(key, 0, true)
		if fresh.aac.valid || fresh.aac.shared || fresh.aac.users != 1 {
			t.Fatal("post-overlap session did not start a fresh baseline")
		}
		fresh.aac.next, fresh.aac.valid = 40, true
		camera.recordEpoch(key, fresh, true, true)
		serial := camera.nextEpoch(key, 0, true)
		if !serial.aac.valid || serial.aac.next != 40 {
			t.Fatal("fresh baseline did not authorize the next serial reconnect")
		}
		camera.recordEpoch(key, serial, true, false)
	}
}

func TestAACSequenceCarryAndWrap(t *testing.T) {
	packet := testAAC(0)
	config, ok := aacConfig(packet.Data)
	if !ok {
		t.Fatal("invalid test AAC")
	}
	m := mediaPipeline{epoch: mediaEpoch{aac: aacSequence{next: 65535, config: config, valid: true}}}
	if err := m.configureAudio(packet); err != nil {
		t.Fatal(err)
	}
	ssrc := m.audioSSRC
	for _, sequence := range []uint16{65535, 0} {
		frame, err := m.frame(packet)
		if err != nil || !frame.valid() || frame.packet.SequenceNumber != sequence || frame.packet.SSRC != ssrc {
			t.Fatalf("AAC sequence/SSRC changed incorrectly: frame=%v err=%v", frame, err)
		}
	}
}
