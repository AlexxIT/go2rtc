package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type CodecHandler interface {
	ProcessPacket(packet *Packet)
	SendCacheTo(sender *Sender)
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

	isKeyframeFunc func([]byte) bool
	// repairKeyframe       func(payload []byte) []byte
	createRTPDepayFunc   func(*Codec, HandlerFunc) HandlerFunc
	createAVCCRepairFunc func(*Codec, HandlerFunc) HandlerFunc
	payloader            Payloader
}

type PreparedFrame struct {
	originalPacket *Packet
	payloads       [][]byte
	timestamp      uint32
}

func NewCodecHandler(
	codec *Codec,
	isKeyframeFunc func([]byte) bool,
	// repairKeyframe func(payload []byte) []byte,
	createRTPDepayFunc func(*Codec, HandlerFunc) HandlerFunc,
	createAVCCRepairFunc func(*Codec, HandlerFunc) HandlerFunc,
	payloader Payloader,
) CodecHandler {
	ch := &BaseCodecHandler{
		codec:          codec,
		isKeyframeFunc: isKeyframeFunc,
		// repairKeyframe:       repairKeyframe,
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
		ch.inputHandler = ch.createAVCCRepairFunc(ch.codec, gopHandler)
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

func (ch *BaseCodecHandler) SendCacheTo(sender *Sender) {
	fmt.Printf("[HANDLER] Sending cache to sender %d for codec %s\n", sender.id, ch.codec.Name)

	ch.mu.RLock()
	cache := ch.gopCache
	ch.mu.RUnlock()

	if cache == nil || !cache.HasContent() {
		// Cache not initialized or empty
		return
	}

	cachedPackets := cache.Get()
	if len(cachedPackets) == 0 {
		// No AVCC frames or RTP fragments to send
		return
	}

	var avccFrames []*Packet
	var rtpFragments []*Packet

	for _, pkt := range cachedPackets {
		if pkt.Header.Version == 0 {
			// RTPPacketVersionAVC = AVCC-Frame
			avccFrames = append(avccFrames, pkt)
		} else {
			// RTP-Fragment
			rtpFragments = append(rtpFragments, pkt)
		}
	}

	// if ch.repairKeyframe != nil {
	// 	for _, pkt := range avccFrames {
	// 		// Add parameter sets to keyframes if needed
	// 		pkt.Payload = ch.repairKeyframe(pkt.Payload)
	// 	}
	// }

	const playbackFPS = 100
	const msPerFrame = 1000 / playbackFPS
	const ticksPerMs = 90000 / 1000
	const ticksPerFrame = ticksPerMs * msPerFrame

	var lastTimestamp uint32
	if len(avccFrames) > 0 {
		// Use the timestamp of the last AVCC frame as the base
		lastTimestamp = avccFrames[len(avccFrames)-1].Header.Timestamp
	} else {
		// Use the timestamp of the last RTP fragment as the base
		lastTimestamp = rtpFragments[len(rtpFragments)-1].Header.Timestamp
	}

	startTime := time.Now()
	delta := -int64(ticksPerMs * len(avccFrames) * msPerFrame)

	fmt.Printf("[HANDLER] Found %d AVCC frames + %d RTP fragments for sender %d\n",
		len(avccFrames), len(rtpFragments), sender.id)

	if !ch.codec.IsRTP() {
		fmt.Printf("[HANDLER] Sending %d AVCC frames directly to sender %d\n",
			len(avccFrames), sender.id)

		for i, pkt := range avccFrames {
			delta += ticksPerFrame
			frameTime := startTime.Add(time.Duration(i*msPerFrame) * time.Millisecond)
			pkt.Header.Timestamp = uint32(int64(lastTimestamp) + delta)
			fmt.Printf("[HANDLER] Sender %d processing cached avcc frame: sequence=%d, timestamp=%d, len=%d\n",
				sender.id, pkt.Header.SequenceNumber, pkt.Header.Timestamp, len(pkt.Payload))
			sender.InputCache(pkt)
			time.Sleep(time.Until(frameTime))
		}
		return
	}

	mtu := uint16(1460)
	preparedFrames := make([]PreparedFrame, 0, len(avccFrames))
	totalRTPPackets := 0

	for _, pkt := range avccFrames {
		delta += ticksPerFrame
		frameTimestamp := uint32(int64(lastTimestamp) + delta)
		payloads := ch.payloader.Payload(mtu, pkt.Payload)
		totalRTPPackets += len(payloads)

		preparedFrames = append(preparedFrames, PreparedFrame{
			originalPacket: pkt,
			payloads:       payloads,
			timestamp:      frameTimestamp,
		})
	}

	totalRTPPackets += len(rtpFragments)

	var cacheEndSequence uint16
	if len(rtpFragments) > 0 {
		cacheEndSequence = rtpFragments[len(rtpFragments)-1].Header.SequenceNumber
	} else {
		cacheEndSequence = avccFrames[len(avccFrames)-1].Header.SequenceNumber
	}

	cacheStartSequence := cacheEndSequence - uint16(totalRTPPackets) + 1

	fmt.Printf("[HANDLER] Sending %d AVCC + %d RTP -> %d total RTP packets, seq %d-%d to sender %d\n",
		len(avccFrames), len(rtpFragments), totalRTPPackets, cacheStartSequence, cacheEndSequence, sender.id)

	currentSequence := cacheStartSequence
	sentPackets := 0
	timePerFrame := time.Second / time.Duration(playbackFPS)

	for _, frame := range preparedFrames {
		last := len(frame.payloads) - 1
		if last < 0 {
			continue
		}

		sleepPerPacket := timePerFrame / time.Duration(len(frame.payloads))

		fmt.Printf("[HANDLER] Processing frame with %d payloads, sleep per frame: %v, sleep per packet: %v\n",
			len(frame.payloads), timePerFrame, sleepPerPacket)

		for j, payload := range frame.payloads {
			sentPackets++

			rtpPacket := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         j == last,
					PayloadType:    frame.originalPacket.Header.PayloadType,
					SequenceNumber: currentSequence,
					SSRC:           frame.originalPacket.Header.SSRC,
					Timestamp:      frame.timestamp,
				},
				Payload: payload,
			}

			fmt.Printf("[HANDLER] Sender %d processing cached avcc frame: sequence=%d, timestamp=%d, len=%d, sleep=%v\n",
				sender.id, rtpPacket.Header.SequenceNumber, rtpPacket.Header.Timestamp, len(rtpPacket.Payload), sleepPerPacket)

			sender.InputCache(rtpPacket)
			currentSequence++
			time.Sleep(sleepPerPacket)
		}
	}

	for _, pkt := range rtpFragments {
		sentPackets++
		pkt.Header.SequenceNumber = currentSequence

		fmt.Printf("[HANDLER] Sender %d processing cached rtp fragment: sequence=%d, timestamp=%d, len=%d, sleep=1ms\n",
			sender.id, pkt.Header.SequenceNumber, pkt.Header.Timestamp, len(pkt.Payload))

		sender.InputCache(pkt)
		currentSequence++
		time.Sleep(1 * time.Millisecond)
	}

	fmt.Printf("[HANDLER] COMPLETE: RTP seq %d-%d (%d packets) for sender %d\n",
		cacheStartSequence, currentSequence-1, sentPackets, sender.id)
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
