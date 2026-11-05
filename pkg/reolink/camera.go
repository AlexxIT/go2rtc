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
	video     uint64
	audio     uint64
	audioRate uint32
	videoSet  bool
	audioSet  bool
}

func (c *Camera) nextEpoch(key ProfileKey, now uint64) mediaEpoch {
	epoch := c.epochs[key]
	epoch.video = nextTimestampEpoch(now, epoch.video, epoch.videoSet)
	epoch.videoSet = true
	return epoch
}

func (c *Camera) recordEpoch(key ProfileKey, value mediaEpoch) {
	previous := c.epochs[key]
	if value.videoSet && (!previous.videoSet || value.video > previous.video) {
		previous.video = value.video
		previous.videoSet = true
	}
	if value.audioSet && (!previous.audioSet || value.audio > rescaleTimestamp(previous.audio, previous.audioRate, value.audioRate)) {
		previous.audio = value.audio
		previous.audioRate = value.audioRate
		previous.audioSet = true
	}
	c.epochs[key] = previous
}

func nextTimestampEpoch(now, previous uint64, set bool) uint64 {
	if set && now <= previous {
		return previous + 1
	}
	return now
}

// Split the product to avoid intermediate overflow for representable results.
func rescaleTimestamp(value uint64, oldRate, newRate uint32) uint64 {
	return value/uint64(oldRate)*uint64(newRate) + value%uint64(oldRate)*uint64(newRate)/uint64(oldRate)
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
