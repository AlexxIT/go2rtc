package h264

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAVC = 0

const PSMaxSize = 128 // the biggest SPS I've seen is 48 (EZVIZ CS-CV210)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	depack := &codecs.H264Packet{IsAVC: true}

	sps, pps := GetParameterSet(track.Codec.FmtpLine)
	ps := EncodeAVC(sps, pps)

	buf := make([]byte, 0, 512*1024) // 512K

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//log.Printf("[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, %v", track.Codec.Name, packet.Payload[0]&0x1F, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

			payload, err := depack.Unmarshal(packet.Payload)
			if len(payload) == 0 || err != nil {
				return nil
			}

			// Fix TP-Link Tapo TC70: sends SPS and PPS with packet.Marker = true
			// Reolink Duo 2: sends SPS with Marker and PPS without
			if packet.Marker && len(payload) < PSMaxSize {
				switch NALUType(payload) {
				case NALUTypeSPS, NALUTypePPS:
					buf = append(buf, payload...)
					return nil
				case NALUTypeSEI:
					// RtspServer https://github.com/AlexxIT/go2rtc/issues/244
					// sends, marked SPS, marked PPS, marked SEI, marked IFrame
					return nil
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
							return nil
						}

						payload = payload[i:]
						continue
					}
					break
				}
			}

			// collect all NALs for Access Unit
			if !packet.Marker {
				buf = append(buf, payload...)
				return nil
			}

			if len(buf) > 0 {
				payload = append(buf, payload...)
				buf = buf[:0]
			}

			// should not be that huge SPS
			if NALUType(payload) == NALUTypeSPS && binary.BigEndian.Uint32(payload) >= PSMaxSize {
				// some Chinese buggy cameras has single packet with SPS+PPS+IFrame separated by 00 00 00 01
				// https://github.com/AlexxIT/WebRTC/issues/391
				// https://github.com/AlexxIT/WebRTC/issues/392
				AnnexB2AVC(payload)
			}

			//log.Printf("[AVC] %v, len: %d, ts: %10d, seq: %d", Types(payload), len(payload), packet.Timestamp, packet.SequenceNumber)

			clone := *packet
			clone.Version = RTPPacketVersionAVC
			clone.Payload = payload
			return push(&clone)
		}
	}
}

func RTPPay(mtu uint16) streamer.WrapperFunc {
	payloader := &Payloader{IsAVC: true}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version != RTPPacketVersionAVC {
				return push(packet)
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
				if err := push(&clone); err != nil {
					return err
				}
			}

			return nil
		}
	}
}
