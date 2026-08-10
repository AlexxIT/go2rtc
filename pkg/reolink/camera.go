package reolink

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

type ProfileKey struct {
	Channel uint8
	Stream  baichuan.Stream
}

type Camera struct {
	registry      *Registry
	key           cameraKey
	remote        string
	profiles      map[*Profile]struct{}
	epochs        map[ProfileKey]mediaEpoch
	refs          int
	generation    uint64
	mainSessions  int
	otherSessions int
}

type mediaEpoch struct {
	video     uint32
	audio     uint32
	audioRate uint32
	videoSet  bool
	audioSet  bool
}

func (c *Camera) nextEpoch(key ProfileKey, now uint32) mediaEpoch {
	epoch := c.epochs[key]
	epoch.video = nextTimestampEpoch(now, epoch.video, epoch.videoSet)
	epoch.videoSet = true
	return epoch
}

func (c *Camera) recordEpoch(key ProfileKey, value mediaEpoch) {
	previous := c.epochs[key]
	if value.videoSet && (!previous.videoSet || timestampAfter(value.video, previous.video)) {
		previous.video = value.video
		previous.videoSet = true
	}
	if value.audioSet && (!previous.audioSet || value.audioRate != previous.audioRate ||
		timestampAfter(value.audio, previous.audio)) {
		previous.audio = value.audio
		previous.audioRate = value.audioRate
		previous.audioSet = true
	}
	c.epochs[key] = previous
}

func timestampAfter(value, previous uint32) bool {
	delta := value - previous
	return delta != 0 && delta < 1<<31
}

func nextTimestampEpoch(now, previous uint32, set bool) uint32 {
	if set && !timestampAfter(now, previous) {
		return previous + 1
	}
	return now
}

func (c *Camera) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "reolink.Camera{Remote:%q}", c.remote)
}

func (c *Camera) addSession(stream baichuan.Stream) {
	if stream == baichuan.StreamMain {
		c.mainSessions++
		return
	}
	c.otherSessions++
}

func (c *Camera) removeSession(stream baichuan.Stream) {
	if stream == baichuan.StreamMain {
		if c.mainSessions == 0 {
			panic("reolink: main session accounting underflow")
		}
		c.mainSessions--
		return
	}
	if c.otherSessions == 0 {
		panic("reolink: other session accounting underflow")
	}
	c.otherSessions--
}
