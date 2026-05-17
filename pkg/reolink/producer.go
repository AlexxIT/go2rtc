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
	timeout := time.After(core.ProbeTimeout)

	for vcodec == nil || acodec == nil {
		select {
		case <-timeout:
			if vcodec == nil {
				return fmt.Errorf("reolink: probe timeout waiting for video iframe")
			}
			c.logDebug("probe timeout, got video but no audio")
			goto DoneProbe
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
			} else if packet.Kind == baichuan.MediaPacketAAC && acodec == nil {
				if aac.IsADTS(packet.Data) {
					acodec = aac.ADTSToCodec(packet.Data)
					c.logDebug("probe got AAC (ADTS) rate=%d ch=%d fmtp=%s", acodec.ClockRate, acodec.Channels, acodec.FmtpLine)
				} else {
					config := aac.EncodeConfig(aac.TypeAACLC, 16000, 1, false)
					acodec = aac.ConfigToCodec(config)
					c.logDebug("probe got AAC (raw) rate=%d", acodec.ClockRate)
				}
			}
		}
	}
DoneProbe:
	c.medias = []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}

	if acodec != nil {
		acodec.PayloadType = core.PayloadTypeRAW
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

	c.logDebug("probe complete, video=%s audio=%v medias=%d", vcodec.Name, acodec != nil, len(c.medias))
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
			return c.bc.Err()
		}

		c.processPacket(packet, &videoCount, &audioCount)
	}
}

func (c *Client) processPacket(packet baichuan.MediaPacket, videoCount, audioCount *int) {
	c.recv += len(packet.Data)

	switch packet.Kind {
	case baichuan.MediaPacketIFrame, baichuan.MediaPacketPFrame:
		if !packet.HasTimestamp {
			return
		}

		if packet.Codec == "H265" {
			if c.probeIFrame != nil && c.probeIFrame.Codec == packet.Codec {
				packet = *c.probeIFrame
				c.probeIFrame = nil
			}

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
		rawVideoRTP := uint32(rtpTimestampForClock(continuousUS, 90000))
		timestamp := c.videoRTP.next(rawVideoRTP)


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

		if packet.HasTimestamp {
			continuousUS := c.videoTimestamps.unwrap(packet.TimestampMicrosecs)
			c.audioSamples = rtpTimestampForClock(continuousUS, int(clockRate))
		} else if c.audioSamples == 0 && *audioCount == 0 {
			nowUS := uint64(time.Now().UnixMicro())
			c.audioSamples = rtpTimestampForClock(nowUS, int(clockRate))
		}

		for _, pkt := range pkts {
			rawAudioRTP := uint32(c.audioSamples)
			pkt.Timestamp = c.audioRTP.next(rawAudioRTP)
			c.audioSamples += 1024 // AAC frame size
		}

		for _, receiver := range c.receivers {
			if receiver.Codec.Name == core.CodecAAC {
				for _, pkt := range pkts {
					clone := *pkt
					receiver.WriteRTP(&clone)
				}
			}
		}

		*audioCount += len(pkts)
		if *audioCount <= 3 && len(pkts) > 0 {
			c.logDebug("audio pkt #%d len=%d ts=%d",
				*audioCount, len(pkts[0].Payload), pkts[0].Timestamp)
		}
	}
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
	offset  uint64
	baseSet bool
}

func (u *timestampUnwrapper) unwrap(ts32 uint32) uint64 {
	if !u.baseSet {
		systemMicro := uint64(time.Now().UnixMicro())
		u.offset = systemMicro - uint64(ts32)
		u.highest = uint64(ts32)
		u.baseSet = true
		return systemMicro
	}

	continuous := unwrapTimestamp(ts32, u.highest)
	if continuous > u.highest {
		u.highest = continuous
	}
	return continuous + u.offset
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
	offset uint32
	last   uint32
	set    bool
}

func (g *rtpTimestampGuard) next(ts uint32) uint32 {
	if !g.set {
		g.last = ts
		g.set = true
		return ts
	}
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

func rtpTimestampForClock(microseconds uint64, clockRate int) uint64 {
	seconds := microseconds / 1_000_000
	rem := microseconds % 1_000_000
	return seconds*uint64(clockRate) + (rem*uint64(clockRate))/1_000_000
}
