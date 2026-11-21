package h264

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAVC = 0

const PSMaxSize = 128 // the biggest SPS I've seen is 48 (EZVIZ CS-CV210)

func RTPDepay(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	depack := &codecs.H264Packet{IsAVC: true}

	sps, pps := GetParameterSet(codec.FmtpLine)
	ps := JoinNALU(sps, pps)

	buf := make([]byte, 0, 512*1024) // 512K

	// pion's depacketizer appends every FU-A fragment to its buffer whether or
	// not it saw the start bit, so a consumer that attaches mid-NAL (or loses
	// the start fragment) would get a synthesized NAL header stapled onto the
	// tail of a slice. Track fragment state and drop such tails ourselves.
	inFragment := false

	// RTP over WebRTC is lossy. A packet missing from the middle of a
	// fragmented IDR leaves a NAL with a hole in it; a decoder renders what it
	// has and smears the rest, and every P-frame after it references the
	// damage. Watch the sequence numbers: on a gap discard whatever is being
	// assembled and pass nothing on until the next access unit that begins
	// with a keyframe received in full.
	var lastSeq uint16
	haveSeq := false
	waitKeyframe := false

	return func(packet *rtp.Packet) {
		if haveSeq && packet.SequenceNumber != lastSeq+1 {
			depack = &codecs.H264Packet{IsAVC: true}
			inFragment = false
			buf = buf[:0]
			waitKeyframe = true
		}
		lastSeq = packet.SequenceNumber
		haveSeq = true

		//log.Printf("[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, %v", codec.Name, packet.Payload[0]&0x1F, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		if len(packet.Payload) >= 2 && packet.Payload[0]&0x1F == NALUTypeFUA {
			start := packet.Payload[1]&0x80 != 0
			end := packet.Payload[1]&0x40 != 0
			switch {
			case start:
				if inFragment {
					// previous fragment set never finished: discard its partial data
					depack = &codecs.H264Packet{IsAVC: true}
				}
			case !inFragment:
				return // tail of a NAL whose start we never saw
			}
			inFragment = !end
		}

		payload, err := depack.Unmarshal(packet.Payload)
		if len(payload) == 0 || err != nil {
			return
		}

		// Memory overflow protection. Can happen if we miss a lot of packets with the marker.
		// https://github.com/AlexxIT/go2rtc/issues/675
		if len(buf) > 5*1024*1024 {
			buf = buf[: 0 : 512*1024]
		}

		// Fix TP-Link Tapo TC70: sends SPS and PPS with packet.Marker = true
		// Reolink Duo 2: sends SPS with Marker and PPS without
		if packet.Marker && len(payload) < PSMaxSize {
			switch NALUType(payload) {
			case NALUTypeSPS, NALUTypePPS:
				buf = append(buf, payload...)
				return
			case NALUTypeSEI:
				// RtspServer https://github.com/AlexxIT/go2rtc/issues/244
				// sends, marked SPS, marked PPS, marked SEI, marked IFrame
				return
			}
		}

		if len(buf) == 0 {
			for {
				// Amcrest IP4M-1051: 9, 7, 8, 6, 28...
				// Amcrest IP4M-1051: 9, 6, 1
				switch NALUType(payload) {
				case NALUTypeIFrame:
					// fix IFrame without SPS,PPS
					buf = append(buf, ps...)
				case NALUTypeSEI, NALUTypeAUD:
					// fix ffmpeg with transcoding first frame
					i := int(4 + binary.BigEndian.Uint32(payload))

					// check if only one NAL (fix ffmpeg transcoding for Reolink RLC-510A)
					if i == len(payload) {
						return
					}

					payload = payload[i:]
					continue
				case NALUTypePFrame, NALUTypeSPS, NALUTypePPS: // pass
				default:
					return // skip any unknown NAL unit type
				}
				break
			}
		}

		// collect all NALs for Access Unit
		if !packet.Marker {
			buf = append(buf, payload...)
			return
		}

		if len(buf) > 0 {
			payload = append(buf, payload...)
			buf = buf[:0]
		}

		// should not be that huge SPS
		if NALUType(payload) == NALUTypeSPS && binary.BigEndian.Uint32(payload) >= PSMaxSize {
			// some Chinese buggy cameras have a single packet with SPS+PPS+IFrame separated by 00 00 00 01
			// https://github.com/AlexxIT/WebRTC/issues/391
			// https://github.com/AlexxIT/WebRTC/issues/392
			payload = annexb.FixAnnexBInAVCC(payload)
		}

		//log.Printf("[AVC] %v, len: %d, ts: %10d, seq: %d", NALUTypes(payload), len(payload), packet.Timestamp, packet.SequenceNumber)

		if waitKeyframe {
			if !IsKeyframe(payload) {
				return
			}
			waitKeyframe = false
		}

		clone := *packet
		clone.Version = RTPPacketVersionAVC
		clone.Payload = payload
		handler(&clone)
	}
}

func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &Payloader{IsAVC: true}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != RTPPacketVersionAVC {
			handler(packet)
			return
		}

		payloads := payloader.Payload(mtu, packet.Payload)
		last := len(payloads) - 1
		for i, payload := range payloads {
			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         i == last,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: payload,
			}
			handler(&clone)
		}
	}
}
