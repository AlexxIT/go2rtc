package reolink

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) Probe() error {
	c.logDebug("probing stream")

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	reader, err := c.bc.StartPreview(ctx, c.channel, c.stream)
	if err != nil {
		return fmt.Errorf("reolink: start preview for probe failed: %w", err)
	}
	c.reader = reader

	var vcodec, acodec *core.Codec
	var iframeTries int
	// Use a 10s timeout instead of core.ProbeTimeout (5s) because long-GOP cameras (e.g. Trackmix)
	// have 2-4s keyframe intervals, and any packet drop or missing VPS/SPS can require waiting
	// for a second keyframe, which would exceed a 5s timeout and cause random loading failures.
	timeout := time.After(10 * time.Second)

ProbeLoop:
	for (c.videoEnabled && vcodec == nil) || (c.audioEnabled && acodec == nil) {
		select {
		case <-timeout:
			if c.videoEnabled && vcodec == nil {
				return fmt.Errorf("reolink: probe timeout waiting for video iframe")
			}
			c.logDebug("probe timeout, proceeding with available codecs")
			break ProbeLoop
		case packet, ok := <-reader.Packets:
			if !ok {
				return c.bc.Err()
			}

			if packet.Kind == baichuan.MediaPacketIFrame && vcodec == nil {
				c.logDebug("probe got iframe codec=%s len=%d tries=%d", packet.Codec, len(packet.Data), iframeTries)
				saved := packet
				c.probeIFrame = &saved
				iframeTries++
				if packet.Codec == "H265" {
					nalus := splitAnnexB(packet.Data)
					nalus = filterH265DecodableNALs(nalus)
					nalus = reorderH265NALsForAccessUnit(nalus)

					var b []byte
					for _, n := range nalus {
						b = append(b, 0, 0, 0, 1)
						b = append(b, n...)
					}

					buf := annexb.EncodeToAVCC(b)
					if len(buf) >= 5 && h265.NALUType(buf) == h265.NALUTypeVPS {
						vcodec = h265.AVCCToCodec(buf)
						c.logDebug("probe H265 fmtp=%s", vcodec.FmtpLine)
					} else if iframeTries < 3 {
						c.logDebug("probe H265 iframe missing VPS, waiting for next one")
						c.probeIFrame = nil
					} else {
						c.logDebug("probe H265 iframe missing VPS, using bare codec")
						vcodec = &core.Codec{Name: core.CodecH265, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
					}
				} else {
					buf := annexb.EncodeToAVCC(packet.Data)
					if len(buf) >= 5 && h264.NALUType(buf) == h264.NALUTypeSPS {
						vcodec = h264.AVCCToCodec(buf)
						c.logDebug("probe H264 fmtp=%s", vcodec.FmtpLine)
					} else if iframeTries < 3 {
						c.logDebug("probe H264 iframe missing SPS, waiting for next one")
						c.probeIFrame = nil
					} else {
						c.logDebug("probe H264 iframe missing SPS, using bare codec")
						vcodec = &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
					}
				}
			} else if (packet.Kind == baichuan.MediaPacketAAC || packet.Kind == baichuan.MediaPacketADPCM) && acodec == nil {
				if packet.Kind == baichuan.MediaPacketAAC {
					if aac.IsADTS(packet.Data) {
						acodec = aac.ADTSToCodec(packet.Data)
						c.logDebug("probe got AAC (ADTS) rate=%d ch=%d fmtp=%s", acodec.ClockRate, acodec.Channels, acodec.FmtpLine)
					} else {
						config := aac.EncodeConfig(aac.TypeAACLC, 16000, 1, false)
						acodec = aac.ConfigToCodec(config)
						c.logDebug("probe got AAC (raw) rate=%d", acodec.ClockRate)
					}
				} else {
					acodec = &core.Codec{
						Name:      core.CodecPCMA,
						ClockRate: 8000,
						Channels:  1,
					}
					c.logDebug("probe got ADPCM -> PCMA rate=8000 ch=1")
				}
			}
		}
	}
	c.medias = []*core.Media{}

	if c.videoEnabled && vcodec != nil {
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		})
	}

	if c.audioEnabled && acodec != nil {
		if acodec.Name == core.CodecAAC {
			acodec.PayloadType = core.PayloadTypeRAW
		}
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		})
	}

	if c.stream != baichuan.StreamMain {
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCMA, ClockRate: 8000},
				{Name: core.CodecPCMU, ClockRate: 8000},
				{Name: core.CodecPCML, ClockRate: 16000},
				{Name: core.CodecPCML, ClockRate: 8000},
			},
		})
	}

	vcodecName := "disabled"
	if vcodec != nil {
		vcodecName = vcodec.Name
	}
	c.logDebug("probe complete, video=%s audio=%v medias=%d", vcodecName, acodec != nil, len(c.medias))
	return nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track := core.NewReceiver(media, codec)
	c.receivers = append(c.receivers, track)

	return track, nil
}

func (c *Client) Start() error {
	c.logDebug("Start() called, receivers=%d reader=%v", len(c.receivers), c.reader != nil)

	if len(c.receivers) == 0 {
		c.logDebug("no receivers, backchannel-only stream started")
		ch := make(chan struct{})
		c.cancel = func() {
			close(ch)
		}
		<-ch
		return nil
	}

	if c.reader == nil {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel

		reader, err := c.bc.StartPreview(ctx, c.channel, c.stream)
		if err != nil {
			return err
		}
		c.reader = reader
	}

	var videoCount, audioCount int

	// replay the I-Frame that was consumed during Probe
	if c.probeIFrame != nil {
		c.processPacket(*c.probeIFrame, &videoCount, &audioCount)
		c.probeIFrame = nil
	}

	for {
		packet, ok := <-c.reader.Packets
		if !ok {
			err := c.bc.Err()
			c.logWarn("camera disconnected: %v, attempting to reconnect", err)
			return err
		}

		c.processPacket(packet, &videoCount, &audioCount)
	}
}

func (c *Client) processPacket(packet baichuan.MediaPacket, videoCount, audioCount *int) {
	c.recv += len(packet.Data)

	switch packet.Kind {
	case baichuan.MediaPacketIFrame, baichuan.MediaPacketPFrame:
		if !c.videoEnabled {
			return
		}
		if !packet.HasTimestamp {
			return
		}


		if packet.Codec == "H265" {
			// NALU reordering...
			nalus := splitAnnexB(packet.Data)
			nalus = filterH265DecodableNALs(nalus)
			nalus = reorderH265NALsForAccessUnit(nalus)

			if len(nalus) == 0 {
				return
			}

			var buf []byte
			for _, n := range nalus {
				buf = append(buf, 0, 0, 0, 1)
				buf = append(buf, n...)
			}
			packet.Data = buf
		}

		packet.Data = annexb.EncodeToAVCC(packet.Data)

		continuousUS := c.videoTimestamps.unwrap(packet.TimestampMicrosecs)

		if !c.baseSet {
			c.baseTicks = continuousUS
			c.baseTime = time.Now()
			c.baseSet = true
		} else if c.baseTicks == 0 {
			// Audio started first. Initialize baseTicks based on elapsed time.
			elapsedUS := uint64(time.Since(c.baseTime).Microseconds())
			if continuousUS > elapsedUS {
				c.baseTicks = continuousUS - elapsedUS
			} else {
				c.baseTicks = 0
			}
		}

		if continuousUS < c.baseTicks {
			// Clock jumped backward or reset below baseTicks. Realize new base to prevent uint64 underflow.
			c.baseTicks = continuousUS
		}

		relativeUS := continuousUS - c.baseTicks

		if relativeUS < c.lastVideoUS {
			// Clock jumped backward. Realign baseTime to match the new timeline and preserve pacing.
			c.baseTime = time.Now().Add(-time.Duration(relativeUS) * time.Microsecond)
		}

		rawVideoRTP := uint32(relativeUS * 90000 / 1_000_000)
		timestamp := c.videoRTP.next(rawVideoRTP)
		c.lastVideoUS = relativeUS

		c.lastWriteTime = time.Now()

		pkt := &core.Packet{
			Header: rtp.Header{
				Marker:    true,
				Timestamp: timestamp,
			},
			Payload: packet.Data,
		}

		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecH264 || receiver.Codec.Name == core.CodecH265 {
				if receiver.Codec.Name == core.CodecH264 && packet.Codec != "H264" {
					continue
				}
				if receiver.Codec.Name == core.CodecH265 && packet.Codec != "H265" {
					continue
				}

				clone := *pkt
				receiver.WriteRTP(&clone)
			}
		}
		*videoCount++

		if *videoCount <= 3 {
			c.logDebug("video pkt #%d codec=%s kind=%d len=%d ts=%d",
				*videoCount, packet.Codec, packet.Kind, len(pkt.Payload), pkt.Timestamp)
		}
	case baichuan.MediaPacketAAC:
		if !c.audioEnabled {
			return
		}
		var pkts []*core.Packet
		payload := packet.Data
		for len(payload) > 0 {
			if !aac.IsADTS(payload) {
				break // reached padding or invalid data
			}

			headerLen := aac.ADTSHeaderLen(payload)
			// Frame length is 13 bits starting at byte 3, bit 13.
			frameLen := (int(payload[3]&3) << 11) | (int(payload[4]) << 3) | (int(payload[5]) >> 5)

			if frameLen < headerLen || frameLen > len(payload) {
				break // invalid frame length
			}

			rawAAC := payload[headerLen:frameLen]

			pkt := &core.Packet{
				Header: rtp.Header{
					Version: aac.RTPPacketVersionAAC,
					Marker:  true,
				},
				Payload: rawAAC,
			}
			pkts = append(pkts, pkt)
			payload = payload[frameLen:]
		}

		if len(pkts) == 0 {
			return
		}

		var clockRate uint32 = 16000
		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecAAC {
				clockRate = receiver.Codec.ClockRate
				break
			}
		}

		if *audioCount == 0 {
			if !c.baseSet {
				c.baseTime = time.Now()
				c.baseSet = true
				c.audioSamples = 0
			} else if c.baseTicks != 0 && time.Since(c.lastWriteTime) < 500*time.Millisecond {
				c.audioSamples = c.guardedVideoUS() * uint64(clockRate) / 1_000_000
			} else {
				elapsed := time.Since(c.baseTime)
				c.audioSamples = uint64(elapsed.Microseconds()) * uint64(clockRate) / 1_000_000
			}
		}



		var outPkts []*core.Packet
		for _, pkt := range pkts {
			if c.baseSet {
				var targetUS uint64
				if c.baseTicks != 0 && c.lastVideoUS != 0 && time.Since(c.lastWriteTime) < 500*time.Millisecond {
					targetUS = c.guardedVideoUS()
				} else {
					targetUS = uint64(time.Since(c.baseTime).Microseconds())
				}

				expectedAudioUS := c.audioSamples * 1_000_000 / uint64(clockRate)
				driftUS := int64(expectedAudioUS) - int64(targetUS)
				driftSamples := driftUS * int64(clockRate) / 1_000_000

				if driftSamples >= 1024 {
					continue // Drop packet
				} else if driftSamples <= -1024 {
					clone1 := *pkt
					clone1.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
					c.audioSamples += 1024
					outPkts = append(outPkts, &clone1)

					clone2 := *pkt
					clone2.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
					c.audioSamples += 1024
					outPkts = append(outPkts, &clone2)
					continue
				}
			}

			pkt.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
			c.audioSamples += 1024
			outPkts = append(outPkts, pkt)
		}

		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecAAC {
				for _, pkt := range outPkts {
					clone := *pkt
					receiver.WriteRTP(&clone)
				}
			}
		}

		*audioCount += len(outPkts)
		if *audioCount <= 3 && len(outPkts) > 0 {
			c.logDebug("audio pkt #%d len=%d ts=%d",
				*audioCount, len(outPkts[0].Payload), outPkts[0].Timestamp)
		}
	case baichuan.MediaPacketADPCM:
		if !c.audioEnabled {
			return
		}
		if c.adpcmDecoder == nil {
			c.adpcmDecoder = &baichuan.ADPCMDecoder{}
		}

		pcm := c.adpcmDecoder.Decode(packet.Data)
		pcma := baichuan.EncodePCMA(pcm)

		if len(pcma) == 0 {
			return
		}

		var clockRate uint32 = 8000
		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecPCMA {
				clockRate = receiver.Codec.ClockRate
				break
			}
		}

		if *audioCount == 0 {
			if !c.baseSet {
				c.baseTime = time.Now()
				c.baseSet = true
				c.audioSamples = 0
			} else if c.baseTicks != 0 && time.Since(c.lastWriteTime) < 500*time.Millisecond {
				c.audioSamples = c.guardedVideoUS() * uint64(clockRate) / 1_000_000
			} else {
				elapsed := time.Since(c.baseTime)
				c.audioSamples = uint64(elapsed.Microseconds()) * uint64(clockRate) / 1_000_000
			}
		}

		var pkts []*core.Packet
		payload := pcma
		for len(payload) > 0 {
			chunkSize := 160
			if len(payload) < chunkSize {
				chunkSize = len(payload)
			}
			chunk := payload[:chunkSize]
			payload = payload[chunkSize:]

			pkt := &core.Packet{
				Header: rtp.Header{
					Marker: true,
				},
				Payload: chunk,
			}
			pkts = append(pkts, pkt)
		}



		var outPkts []*core.Packet
		for _, pkt := range pkts {
			chunkSize := int64(len(pkt.Payload))
			if c.baseSet {
				var targetUS uint64
				if c.baseTicks != 0 && c.lastVideoUS != 0 && time.Since(c.lastWriteTime) < 500*time.Millisecond {
					targetUS = c.guardedVideoUS()
				} else {
					targetUS = uint64(time.Since(c.baseTime).Microseconds())
				}

				expectedAudioUS := c.audioSamples * 1_000_000 / uint64(clockRate)
				driftUS := int64(expectedAudioUS) - int64(targetUS)
				driftSamples := driftUS * int64(clockRate) / 1_000_000

				if driftSamples >= chunkSize {
					continue // Drop packet
				} else if driftSamples <= -chunkSize {
					clone1 := *pkt
					clone1.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
					c.audioSamples += uint64(chunkSize)
					outPkts = append(outPkts, &clone1)

					clone2 := *pkt
					clone2.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
					c.audioSamples += uint64(chunkSize)
					outPkts = append(outPkts, &clone2)
					continue
				}
			}

			pkt.Timestamp = c.audioRTP.next(uint32(c.audioSamples))
			c.audioSamples += uint64(chunkSize)
			outPkts = append(outPkts, pkt)
		}

		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecPCMA {
				for _, pkt := range outPkts {
					clone := *pkt
					receiver.WriteRTP(&clone)
				}
			}
		}

		*audioCount += len(outPkts)
		if *audioCount <= 3 && len(outPkts) > 0 {
			c.logDebug("audio pkt #%d len=%d ts=%d",
				*audioCount, len(outPkts[0].Payload), outPkts[0].Timestamp)
		}
	}
}

func (c *Client) guardedVideoUS() uint64 {
	offsetUS := int64(int32(c.videoRTP.offset)) * 1_000_000 / 90000
	guarded := int64(c.lastVideoUS) + offsetUS
	if guarded < 0 {
		return 0
	}
	return uint64(guarded)
}

func (c *Client) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	if c.sender != nil {
		c.sender.Close()
	}
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: "reolink",
		Protocol:   "baichuan",
		Medias:     c.medias,
		Recv:       c.recv,
		Receivers:  c.receivers,
		Send:       c.send,
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}

type timestampUnwrapper struct {
	highest uint64
	baseSet bool
}

func (u *timestampUnwrapper) unwrap(ts32 uint32) uint64 {
	if !u.baseSet {
		u.highest = uint64(ts32)
		u.baseSet = true
		return uint64(ts32)
	}

	continuous := unwrapTimestamp(ts32, u.highest)
	if continuous > u.highest {
		u.highest = continuous
	}
	return continuous
}



func unwrapTimestamp(ts32 uint32, highest64 uint64) uint64 {
	if highest64 == 0 {
		return uint64(ts32)
	}

	high32 := highest64 >> 32
	cand1 := (high32 << 32) | uint64(ts32)

	cand2 := cand1
	if cand1 >= 0x100000000 {
		cand2 = cand1 - 0x100000000
	}

	cand3 := cand1 + 0x100000000

	absDiff := func(a, b uint64) uint64 {
		if a > b {
			return a - b
		}
		return b - a
	}

	bestCand := cand1
	bestDiff := absDiff(cand1, highest64)

	if diff2 := absDiff(cand2, highest64); diff2 < bestDiff {
		bestCand = cand2
		bestDiff = diff2
	}
	if diff3 := absDiff(cand3, highest64); diff3 < bestDiff {
		bestCand = cand3
	}

	return bestCand
}

type rtpTimestampGuard struct {
	offset   uint32
	last     uint32
	set      bool
	smooth   bool
	avgDelta float64
	lastRaw  uint32
}

func (g *rtpTimestampGuard) next(ts uint32) uint32 {
	if !g.set {
		g.last = ts
		g.lastRaw = ts
		g.avgDelta = 6000 // default for 15 FPS
		g.set = true
		return ts
	}

	if !g.smooth {
		adjusted := ts + g.offset
		if ts == g.last {
			g.offset = g.last + 1 - ts
			adjusted = g.last + 1
		} else if int32(adjusted-g.last) <= 0 {
			jumpBackward := uint32(int32(g.last - adjusted))
			if jumpBackward > 90000 {
				g.offset = g.last + 1 - ts
				adjusted = ts + g.offset
			} else {
				adjusted = g.last + 1
			}
		}
		g.last = adjusted
		return adjusted
	}

	rawDelta := int32(ts - g.lastRaw)
	if rawDelta < 100 || rawDelta > 45000 {
		// Jump or discontinuity (wrap, drop, restart)
		g.lastRaw = ts
		g.avgDelta = 6000

		adjusted := ts + g.offset
		if int32(adjusted-g.last) <= 0 {
			adjusted = g.last + 1
		}
		g.offset = adjusted - ts
		g.last = adjusted
		return adjusted
	}

	// Exponential moving average for average delta
	g.avgDelta = (g.avgDelta*15 + float64(rawDelta)) / 16
	g.lastRaw = ts

	// PLL feedback correction
	step := g.avgDelta
	expected := ts + g.offset
	drift := int32(g.last + uint32(step+0.5) - expected)

	if drift > 9000 { // >100ms ahead of camera -> slow down step
		step -= 200
	} else if drift < -9000 { // >100ms behind camera -> speed up step
		step += 200
	}

	adjusted := g.last + uint32(step+0.5)

	if int32(adjusted-g.last) <= 0 {
		adjusted = g.last + 1
	}

	g.offset = adjusted - ts
	g.last = adjusted
	return adjusted
}
