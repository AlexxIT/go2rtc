package core

import (
	"sync"
	"time"

	"github.com/pion/rtp"
)

type CodecHandler interface {
	ProcessPacket(packet *Packet)
	SendCacheTo(s *Sender, playbackFPS int) (nextTimestamp uint32, lastSequence uint16)
	SendQueueTo(s *Sender, playbackFPS int, startTimestamp uint32, lastSequence uint16)
	ClearCache()
}

type Payloader interface {
	Payload(mtu uint16, payload []byte) [][]byte
}

type BaseCodecHandler struct {
	codec        *Codec
	gopCache     *GopCache
	inputHandler HandlerFunc
	mu           sync.RWMutex

	isKeyframeFunc       func([]byte) bool
	createRTPDepayFunc   func(*Codec, HandlerFunc) HandlerFunc
	createAVCCRepairFunc func(*Codec, HandlerFunc) HandlerFunc
	payloader            Payloader
}

func NewCodecHandler(
	codec *Codec,
	isKeyframeFunc func([]byte) bool,
	createRTPDepayFunc func(*Codec, HandlerFunc) HandlerFunc,
	createAVCCRepairFunc func(*Codec, HandlerFunc) HandlerFunc,
	payloader Payloader,
) CodecHandler {
	ch := &BaseCodecHandler{
		codec:                codec,
		isKeyframeFunc:       isKeyframeFunc,
		createRTPDepayFunc:   createRTPDepayFunc,
		createAVCCRepairFunc: createAVCCRepairFunc,
		payloader:            payloader,
		gopCache:             &GopCache{},
	}

	gopHandler := func(packet *Packet) {
		isKeyframe := ch.isKeyframeFunc(packet.Payload)
		ch.gopCache.Add(packet, isKeyframe)
	}

	if ch.codec.IsRTP() {
		ch.inputHandler = ch.createRTPDepayFunc(ch.codec, gopHandler)
	} else {
		ch.inputHandler = gopHandler
	}

	return ch
}

func (ch *BaseCodecHandler) ProcessPacket(packet *Packet) {
	ch.mu.RLock()
	handler := ch.inputHandler
	cache := ch.gopCache
	ch.mu.RUnlock()

	if cache != nil {
		cache.AddRTPFragment(packet)
	}

	if handler != nil {
		handler(packet)
	}
}

func (ch *BaseCodecHandler) SendCacheTo(s *Sender, playbackFPS int) (nextTimestamp uint32, lastSequence uint16) {
	// fmt.Printf("[HANDLER] Sending cache to sender %d for codec %s at %d FPS\n", s.id, ch.codec.Name, playbackFPS)

	ch.mu.RLock()
	cache := ch.gopCache
	ch.mu.RUnlock()

	if cache == nil || !cache.HasContent() {
		// fmt.Printf("[HANDLER] No content in cache for sender %d\n", s.id)
		return 0, 0
	}
	cachedPackets := cache.Get()
	if len(cachedPackets) == 0 {
		// fmt.Printf("[HANDLER] No cached packets to send for sender %d\n", s.id)
		return 0, 0
	}

	sleepDurationPerFrame := time.Second / time.Duration(playbackFPS)
	ticksPerPlaybackFrame := uint32(90000 / playbackFPS)
	lastOriginalTimestamp := cachedPackets[len(cachedPackets)-1].Header.Timestamp

	var avccFrames []*Packet
	var rtpFragments []*Packet
	for _, pkt := range cachedPackets {
		if pkt.Header.Version == 0 {
			avccFrames = append(avccFrames, pkt)
		} else {
			rtpFragments = append(rtpFragments, pkt)
		}
	}

	currentTimestamp := lastOriginalTimestamp - uint32(len(avccFrames)*int(ticksPerPlaybackFrame))

	// firstAVCCSequenceNumber := avccFrames[0].Header.SequenceNumber
	// lastAVCCSequenceNumber := firstAVCCSequenceNumber
	// if len(avccFrames) > 1 {
	// 	lastAVCCSequenceNumber = avccFrames[len(avccFrames)-1].Header.SequenceNumber
	// }

	// fmt.Printf("[HANDLER] Sender %d processing %d AVCC frames and %d RTP fragments\n",
	// 	s.id, len(avccFrames), len(rtpFragments))

	// fmt.Printf("[HANDLER] AVCC frames: first sequence=%d, last sequence=%d\n",
	// 	firstAVCCSequenceNumber, lastAVCCSequenceNumber)

	var lastSequenceNumber uint16

	for _, pkt := range avccFrames {
		clone := &rtp.Packet{Header: pkt.Header, Payload: pkt.Payload}
		clone.Header.Timestamp = currentTimestamp
		// fmt.Printf("[HANDLER] Sender %d processing cached avcc frame: sequence=%d, timestamp=%d, len=%d\n",
		// 	s.id, pkt.Header.SequenceNumber, pkt.Header.Timestamp, len(pkt.Payload))
		s.InputCache(clone)
		time.Sleep(sleepDurationPerFrame)
		currentTimestamp += ticksPerPlaybackFrame
		lastSequenceNumber = pkt.Header.SequenceNumber
	}

	if len(rtpFragments) > 0 {
		// firstRTPSequenceNumber := rtpFragments[0].Header.SequenceNumber
		// lastRTPSequenceNumber := firstRTPSequenceNumber
		// if len(rtpFragments) > 1 {
		// 	lastRTPSequenceNumber = rtpFragments[len(rtpFragments)-1].Header.SequenceNumber
		// }

		// fmt.Printf("[HANDLER] RTP fragments: first sequence=%d, last sequence=%d\n",
		// 	firstRTPSequenceNumber, lastRTPSequenceNumber)

		ticksPerFragment := ticksPerPlaybackFrame / uint32(len(rtpFragments))
		sleepDurationPerFragment := sleepDurationPerFrame / time.Duration(len(rtpFragments))

		for _, pkt := range rtpFragments {
			clone := &rtp.Packet{Header: pkt.Header, Payload: pkt.Payload}
			clone.Header.Timestamp = currentTimestamp
			// fmt.Printf("[HANDLER] Sender %d processing cached RTP fragment: sequence=%d, timestamp=%d, len=%d\n",
			// 	s.id, pkt.Header.SequenceNumber, pkt.Header.Timestamp, len(pkt.Payload))
			s.InputCache(clone)
			time.Sleep(sleepDurationPerFragment)
			currentTimestamp += ticksPerFragment
			lastSequenceNumber = pkt.Header.SequenceNumber
		}
	}

	// fmt.Printf("[HANDLER] COMPLETE: Sent %d AVCC frames and %d RTP fragments. Next timestamp: %d\n", len(avccFrames), len(rtpFragments), currentTimestamp)
	return currentTimestamp, lastSequenceNumber
}

func (ch *BaseCodecHandler) SendQueueTo(s *Sender, playbackFPS int, startTimestamp uint32, lastSequence uint16) {
	ticksPerPlaybackFrame := uint32(90000 / playbackFPS)
	currentTimestamp := startTimestamp

	// fmt.Printf("[SENDER] Sender %d starting to process live queue at %d FPS\n", s.id, playbackFPS)

	for {
		select {
		case packet := <-s.liveQueue:
			if packet.Header.SequenceNumber != 0 && packet.Header.SequenceNumber <= lastSequence {
				// fmt.Printf("[SENDER] Sender %d skipping packet with sequence %d (already processed)\n",
				// 	s.id, packet.Header.SequenceNumber)
				continue
			}

			if currentTimestamp == 0 {
				currentTimestamp = packet.Header.Timestamp
			}

			packet.Header.Timestamp = currentTimestamp

			// fmt.Printf("[SENDER] Sender %d processing queued live packet: sequence=%d, timestamp=%d, len=%d\n",
			// 	s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload))

			s.processPacket(packet)

			if packet.Marker {
				currentTimestamp += ticksPerPlaybackFrame
			}

		default:
			// fmt.Printf("[SENDER] COMPLETE: Sender %d finished processing live queue. Switching to live mode.\n", s.id)
			return
		}
	}
}

func (ch *BaseCodecHandler) ClearCache() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.gopCache != nil {
		ch.gopCache.Clear()
	}
}

var (
	codecHandlerFactories = make(map[string]func(*Codec) CodecHandler)
	managerMutex          sync.RWMutex
)

func RegisterCodecHandler(codecName string, factory func(*Codec) CodecHandler) {
	managerMutex.Lock()
	defer managerMutex.Unlock()
	codecHandlerFactories[codecName] = factory
}

func CreateCodecHandler(codec *Codec) CodecHandler {
	managerMutex.RLock()
	factory, exists := codecHandlerFactories[codec.Name]
	managerMutex.RUnlock()

	if !exists {
		return nil
	}

	return factory(codec)
}
