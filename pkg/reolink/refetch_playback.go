package reolink

import (
	"context"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

// The producer is the only reader. Joined teardown remains owned by profileInput.
type refetchInput struct {
	profileInput
	packets     []baichuan.MediaPacket
	raw         uint32
	cadence     refetchCadence
	releasedAt  time.Time
	videoPlayed time.Duration
}

const (
	maxRefetchVideoStep            = uint32(500 * time.Millisecond / time.Microsecond)
	refetchCadenceToleranceDivisor = 8
)

type refetchCadence struct {
	steps []uint32
	next  int
}

func (h *refetchHistory) cadence() refetchCadence {
	fallback := refetchCadence{steps: []uint32{h.step}}
	var intervals [refetchTail - 1]uint32
	steps := intervals[:0]
	var previous uint32
	seenVideo := false
	for i := 0; i < h.count; i++ {
		mark := h.marks[(h.next+refetchTail-h.count+i)%refetchTail]
		if mark.track != trackVideo {
			continue
		}
		if seenVideo {
			step := mark.value - previous
			if step == 0 || step > maxRefetchVideoStep {
				return fallback
			}
			steps = append(steps, step)
		}
		previous, seenVideo = mark.value, true
	}
	for period := 1; 2*period <= len(steps); period++ {
		if !repeatsRefetchSteps(steps, period) {
			continue
		}
		// The most recent cycle starts at the phase expected after the anchor.
		cycle := steps[len(steps)-period:]
		for _, step := range cycle {
			if !sameRefetchStep(step, cycle[len(cycle)-1]) {
				return refetchCadence{steps: cycle}
			}
		}
		break
	}
	return fallback
}

func repeatsRefetchSteps(steps []uint32, period int) bool {
	// Compare each earlier step with the latest step at the same phase.
	last := len(steps) - 1
	for offset := period; offset <= last; offset++ {
		if !sameRefetchStep(steps[last-offset], steps[last-offset%period]) {
			return false
		}
	}
	return true
}

func sameRefetchStep(step, expected uint32) bool {
	return max(step, expected)-min(step, expected) <= expected/refetchCadenceToleranceDivisor
}

func (c *refetchCadence) advance(kind baichuan.MediaKind, step uint32) bool {
	expected := c.steps[c.next]
	if !refetchVideoStep(kind, step, expected) {
		return false
	}
	// A long phase followed by a short phase can fit inside the fallback ceiling.
	if kind == baichuan.MediaVideoP && len(c.steps) > 1 && step > expected+expected/refetchCadenceToleranceDivisor {
		return false
	}
	// An admissible I frame occupies one slot without changing the learned cycle.
	c.next = (c.next + 1) % len(c.steps)
	return true
}

func refetchVideoStep(kind baichuan.MediaKind, step, cadence uint32) bool {
	if step == 0 || step > maxRefetchVideoStep {
		return false
	}
	// A prediction frame can depend on frames that a seek skipped.
	return kind != baichuan.MediaVideoP ||
		cadence > 0 && cadence <= maxRefetchVideoStep && step <= cadence+cadence/2
}

func (i *refetchInput) Read(ctx context.Context) (baichuan.MediaPacket, error) {
	if err := ctx.Err(); err != nil {
		return baichuan.MediaPacket{}, err
	}
	var packet baichuan.MediaPacket
	if len(i.packets) != 0 {
		packet = i.packets[0]
		i.packets[0] = baichuan.MediaPacket{}
		i.packets = i.packets[1:]
	} else {
		var err error
		packet, err = i.profileInput.Read(ctx)
		if err != nil {
			return baichuan.MediaPacket{}, err
		}
	}
	// Cover the requested history and lookup budget during the initial drain.
	if i.videoPlayed >= 2*(refetchSeconds*time.Second+refetchTimeout) {
		return packet, nil
	}
	if packet.Kind == baichuan.MediaDiscontinuity {
		return baichuan.MediaPacket{}, errRefetchContinuity
	}
	if packet.Kind != baichuan.MediaVideoI && packet.Kind != baichuan.MediaVideoP {
		return packet, nil
	}
	step := packet.Timestamp - i.raw
	if !i.cadence.advance(packet.Kind, step) {
		return baichuan.MediaPacket{}, errRefetchContinuity
	}
	i.raw = packet.Timestamp
	advance := time.Duration(step) * time.Microsecond
	i.videoPlayed += advance
	// Space video handoffs without borrowing time from an earlier slow read.
	if delay := time.Until(i.releasedAt.Add(advance / 2)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return baichuan.MediaPacket{}, ctx.Err()
		}
	}
	i.releasedAt = time.Now()
	return packet, nil
}
