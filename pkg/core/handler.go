package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type CodecHandler interface {
	ProcessPacket(packet *Packet)
	SendCacheTo(s *Sender, playbackFPS int) (nextTimestamp uint32)
	SendQueueTo(s *Sender, playbackFPS int, startTimestamp uint32)
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

func (ch *BaseCodecHandler) SendCacheTo(s *Sender, playbackFPS int) uint32 {
	fmt.Printf("[HANDLER] Sending cache to sender %d for codec %s at %d FPS\n", s.id, ch.codec.Name, playbackFPS)

	ch.mu.RLock()
	cache := ch.gopCache
	ch.mu.RUnlock()

	if cache == nil || !cache.HasContent() {
		fmt.Printf("[HANDLER] No content in cache for sender %d\n", s.id)
		return 0
	}
	cachedPackets := cache.Get()
	if len(cachedPackets) == 0 {
		fmt.Printf("[HANDLER] No cached packets to send for sender %d\n", s.id)
		return 0
	}

	sleepDurationPerFrame := time.Second / time.Duration(playbackFPS)
	ticksPerPlaybackFrame := uint32(90000 / playbackFPS)
	lastOriginalTimestamp := cachedPackets[len(cachedPackets)-1].Header.Timestamp

	if !ch.codec.IsRTP() {
		var avccFrames []*Packet
		for _, pkt := range cachedPackets {
			if pkt.Header.Version == 0 {
				avccFrames = append(avccFrames, pkt)
			}
		}

		currentTimestamp := lastOriginalTimestamp - uint32(len(avccFrames)*int(ticksPerPlaybackFrame))
		fmt.Printf("[HANDLER] Sending %d raw AVCC frames to sender %d\n", len(avccFrames), s.id)

		for _, pkt := range avccFrames {
			clone := &rtp.Packet{Header: pkt.Header, Payload: pkt.Payload}
			clone.Header.Timestamp = currentTimestamp
			fmt.Printf("[HANDLER] Sender %d processing cached avcc frame: sequence=%d, timestamp=%d, len=%d\n",
				s.id, pkt.Header.SequenceNumber, pkt.Header.Timestamp, len(pkt.Payload))
			s.InputCache(clone)
			time.Sleep(sleepDurationPerFrame)
			currentTimestamp += ticksPerPlaybackFrame
		}

		fmt.Printf("[HANDLER] COMPLETE: Sent %d AVCC frames. Next timestamp: %d\n", len(avccFrames), currentTimestamp)
		return currentTimestamp
	}

	allRtpPackets := make([]*rtp.Packet, 0, len(cachedPackets))
	var frameCount int

	for i, pkt := range cachedPackets {
		if pkt.Header.Version == 0 {
			// AVCC-Frame
			payloads := ch.payloader.Payload(1460, pkt.Payload)
			last := len(payloads) - 1
			for j, payload := range payloads {
				allRtpPackets = append(allRtpPackets, &rtp.Packet{
					Header: rtp.Header{
						Version:     2,
						Marker:      j == last,
						PayloadType: pkt.Header.PayloadType,
					},
					Payload: payload,
				})

				if j == last {
					frameCount++
				}
			}
		} else {
			// RTP-Paket
			allRtpPackets = append(allRtpPackets, pkt)

			if pkt.Marker {
				frameCount++
			}
		}

		if i == len(cachedPackets)-1 && !pkt.Marker {
			// Last packet in cache is not a marker, tread it as a start of a new frame
			frameCount++
		}
	}

	if len(allRtpPackets) == 0 {
		fmt.Printf("[HANDLER] No RTP packets found in cache for sender %d\n", s.id)
		return 0
	}

	currentTimestamp := lastOriginalTimestamp - uint32(frameCount*int(ticksPerPlaybackFrame))
	cacheEndSequence := allRtpPackets[len(allRtpPackets)-1].Header.SequenceNumber
	currentSequence := cacheEndSequence - uint16(len(allRtpPackets)) + 1

	fmt.Printf("[HANDLER] Found %d RTP packets in cache for sender %d (%d-%d)\n",
		len(allRtpPackets), s.id,
		currentSequence, currentSequence+ uint16(len(allRtpPackets))-1)

	startOfFrame := 0
	for i, packet := range allRtpPackets {
		if packet.Marker || i == len(allRtpPackets)-1 {
			frameSlice := allRtpPackets[startOfFrame : i+1]
			sleepPerPacket := sleepDurationPerFrame / time.Duration(len(frameSlice))

			for _, p := range frameSlice {
				p.Header.SequenceNumber = currentSequence
				p.Header.Timestamp = currentTimestamp
				fmt.Printf("[HANDLER] Sender %d processing cached RTP packet: sequence=%d, timestamp=%d, len=%d\n",
					s.id, p.Header.SequenceNumber, p.Header.Timestamp, len(p.Payload))
				s.InputCache(p)
				time.Sleep(sleepPerPacket)
				currentSequence++
			}

			currentTimestamp += ticksPerPlaybackFrame
			startOfFrame = i + 1
		}
	}

	fmt.Printf("[HANDLER] COMPLETE: Sent %d RTP packets. Next timestamp: %d (sequence=%d)\n",
		len(allRtpPackets), currentTimestamp, currentSequence)

	return currentTimestamp
}

func (ch *BaseCodecHandler) SendQueueTo(s *Sender, playbackFPS int, startTimestamp uint32) {
	ticksPerPlaybackFrame := uint32(90000 / playbackFPS)
	currentTimestamp := startTimestamp

	fmt.Printf("[SENDER] Sender %d starting to process live queue at %d FPS\n", s.id, playbackFPS)

	for {
		select {
		case packet := <-s.liveQueue:
			if currentTimestamp == 0 {
				currentTimestamp = packet.Header.Timestamp
			}

			packet.Header.Timestamp = currentTimestamp

			fmt.Printf("[SENDER] Sender %d processing queued live packet: sequence=%d, timestamp=%d, len=%d\n",
				s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload))

			s.processPacket(packet)

			if packet.Marker {
				currentTimestamp += ticksPerPlaybackFrame
			}

		default:
			fmt.Printf("[SENDER] COMPLETE: Sender %d finished processing live queue. Switching to live mode.\n", s.id)
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
