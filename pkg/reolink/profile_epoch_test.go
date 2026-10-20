package reolink

import (
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestProfileVideoEpochUsesCurrentClock(t *testing.T) {
	for _, name := range []string{"Initial", "Forward"} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			startTS := uint64(start.Unix())*90000 + uint64(start.Nanosecond())*90000/uint64(time.Second)
			key := ProfileKey{}
			camera := &Camera{epochs: make(map[ProfileKey]mediaEpoch)}
			if name == "Forward" {
				camera.epochs[key] = mediaEpoch{video: startTS - 90000, videoSet: true}
			}
			profile := newProfile(camera, key, 1, source{}, nil)
			defer profile.cancel()
			end := time.Now()
			endTS := uint64(end.Unix())*90000 + uint64(end.Nanosecond())*90000/uint64(time.Second)
			actual := profile.pipeline.epoch.video
			if actual-startTS > endTS-startTS {
				t.Fatalf("video epoch outside current clock interval: %d not in %d..%d", actual, startTS, endTS)
			}
		})
	}
}

func TestProfileLongSessionRecordsTerminalEpoch(t *testing.T) {
	key := ProfileKey{}
	const start = uint64(1000000)
	camera := &Camera{
		registry: testRegistry(nil), refs: 1,
		epochs: map[ProfileKey]mediaEpoch{key: {
			video: start, audio: start, audioRate: 48000, videoSet: true, audioSet: true,
		}},
	}
	camera.addSession(key.Stream)
	seconds := uint64(24 * time.Hour / time.Second)
	video := start + seconds*90000
	audio := start + seconds*48000
	profile := &Profile{
		camera: camera, key: key, generation: 2, state: profileFailed,
		pipeline: mediaPipeline{
			videoTS: mediaClock{set: true, value: video},
			audioTS: audio, audioRate: 48000, audioMedia: &core.Media{},
		},
	}
	profile.finishSession(nil)
	epoch := camera.epochs[key]
	if epoch.video != video || epoch.audio != audio || !epoch.videoSet || !epoch.audioSet {
		t.Fatalf("long session terminal epoch was lost: %+v", epoch)
	}
	if next := camera.nextEpoch(key, video); next.video != video+1 {
		t.Fatalf("replacement did not retain terminal epoch: %d != %d", next.video, video+1)
	}
}

func TestProfileAliasesRetainTerminalHighWater(t *testing.T) {
	for _, order := range [][2]int{{1, 0}, {0, 1}} {
		key := ProfileKey{}
		camera := &Camera{registry: testRegistry(nil), refs: 1, epochs: make(map[ProfileKey]mediaEpoch)}
		start := uint64(time.Now().Unix()) + 60
		var profiles [2]*Profile
		for i, seconds := range []uint64{22, 20} {
			camera.addSession(key.Stream)
			profiles[i] = &Profile{
				camera: camera, key: key, generation: uint64(i + 1), state: profileFailed,
				pipeline: mediaPipeline{
					videoTS: mediaClock{set: true, value: (start + seconds) * 90000},
					audioTS: (start + seconds) * 48000, audioRate: 48000, audioMedia: &core.Media{},
				},
			}
		}
		profiles[order[0]].finishSession(nil)
		profiles[order[1]].finishSession(nil)
		epoch := camera.epochs[key]
		if epoch.video != profiles[0].pipeline.videoTS.value || epoch.audio != profiles[0].pipeline.audioTS {
			t.Fatalf("finish order %v lost the later terminal clock: %+v", order, epoch)
		}
		replacement := newProfile(camera, key, 3, source{}, nil)
		replacement.cancel()
		if replacement.pipeline.epoch.video != epoch.video+1 {
			t.Fatalf("finish order %v reconnected behind high water: %+v", order, replacement.pipeline.epoch)
		}
	}
}

func TestCameraEpochAudioRatesAndMissingTracks(t *testing.T) {
	key := ProfileKey{}
	camera := &Camera{epochs: make(map[ProfileKey]mediaEpoch)}
	camera.recordEpoch(key, mediaEpoch{video: 200, videoSet: true})
	if camera.epochs[key].audioSet {
		t.Fatal("missing audio created an epoch")
	}
	camera.recordEpoch(key, mediaEpoch{audio: 48000, audioRate: 48000, audioSet: true})
	for _, test := range []struct {
		value        mediaEpoch
		video, audio uint64
		rate         uint32
	}{
		{mediaEpoch{audio: 44100, audioRate: 44100, audioSet: true}, 200, 48000, 48000},
		{mediaEpoch{video: 300, videoSet: true}, 300, 48000, 48000},
		{mediaEpoch{audio: 22050, audioRate: 44100, audioSet: true}, 300, 48000, 48000},
		{mediaEpoch{audio: 44101, audioRate: 44100, audioSet: true}, 300, 44101, 44100},
		{mediaEpoch{video: 600, videoSet: true}, 600, 44101, 44100},
		{mediaEpoch{audio: 48001, audioRate: 48000, audioSet: true}, 600, 44101, 44100},
	} {
		camera.recordEpoch(key, test.value)
		if epoch := camera.epochs[key]; epoch.video != test.video || epoch.audio != test.audio ||
			epoch.audioRate != test.rate || !epoch.videoSet || !epoch.audioSet {
			t.Fatalf("terminal track high water changed incorrectly: value=%+v epoch=%+v", test.value, epoch)
		}
	}
}

func TestProfileAudioEpochRescalesPreviousClock(t *testing.T) {
	previous := (nowTimestamp(48000)/48000+60)*48000 + 2
	pipeline := mediaPipeline{epoch: mediaEpoch{audio: previous, audioRate: 48000, audioSet: true}}
	if err := pipeline.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	if pipeline.audioRate != 16000 || pipeline.audioTS != previous/3+1 {
		t.Fatalf("cross-rate setup lost terminal high water: rate=%d timestamp=%d", pipeline.audioRate, pipeline.audioTS)
	}
}

func TestMediaEpochKeepsRTPWireWidth(t *testing.T) {
	clock := mediaClock{value: 1<<32 - 100}
	if wire, err := clock.next(1000000, 90000); err != nil || wire != uint32(1<<32-100) {
		t.Fatalf("initial wire timestamp changed: %d %v", wire, err)
	}
	if wire, err := clock.next(1050000, 90000); err != nil || wire != 4400 || clock.value != 1<<32+4400 {
		t.Fatalf("video lost full-width ticks or wire wrap: %d %d %v", wire, clock.value, err)
	}
	pipeline := mediaPipeline{}
	if err := pipeline.configureAudio(testAAC(0)); err != nil {
		t.Fatal(err)
	}
	pipeline.audioTS = 1<<32 - 512
	for _, want := range []uint32{1<<32 - 512, 512} {
		frame, err := pipeline.frame(testAAC(0))
		if err != nil || !frame.valid() || frame.packet.Timestamp != want {
			t.Fatalf("audio wire timestamp changed: frame=%v err=%v", frame, err)
		}
	}
	if pipeline.audioTS != 1<<32+1536 {
		t.Fatalf("audio lost full-width ticks: %d", pipeline.audioTS)
	}
}
