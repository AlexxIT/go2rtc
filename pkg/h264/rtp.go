package h264

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAVC = 0

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	depack := &codecs.H264Packet{IsAVC: true}

	sps, pps := GetParameterSet(track.Codec.FmtpLine)
	sps = EncodeAVC(sps)
	pps = EncodeAVC(pps)

	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//nalUnitType := packet.Payload[0] & 0x1F
			//fmt.Printf(
			//	"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d\n",
			//	track.Codec.Name, nalUnitType, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC, packet.SequenceNumber,
			//)

			data, err := depack.Unmarshal(packet.Payload)
			if len(data) == 0 || err != nil {
				return nil
			}

			for {
				unitType := NALUType(data)
				//fmt.Printf("[H264] nalu: %2d, size: %6d\n", unitType, len(data))

				// multiple 5 and 1 in one payload is OK
				if unitType != NALUTypeIFrame && unitType != NALUTypePFrame {
					i := int(binary.BigEndian.Uint32(data)) + 4
					if i < len(data) {
						data0 := data[:i] // NAL Unit with AVC header
						data = data[i:]
						switch unitType {
						case NALUTypeSPS:
							sps = data0
							continue
						case NALUTypePPS:
							pps = data0
							continue
						case NALUTypeSEI:
							// some unnecessary text information
							continue
						}
					}
				}

				switch unitType {
				case NALUTypeSPS:
					sps = data
					return nil
				case NALUTypePPS:
					pps = data
					return nil
				case NALUTypeSEI:
					// some unnecessary text information
					return nil
				}

				// ffmpeg with `-tune zerolatency` enable option `-x264opts sliced-threads=1`
				// and every NALU will be sliced to multiple NALUs
				if !packet.Marker {
					buffer = append(buffer, data...)
					return nil
				}

				if buffer != nil {
					buffer = append(buffer, data...)
					data = buffer
					buffer = nil
				}

				var clone rtp.Packet

				if unitType == NALUTypeIFrame {
					clone = *packet
					clone.Version = RTPPacketVersionAVC
					clone.Payload = sps
					if err = push(&clone); err != nil {
						return err
					}

					clone = *packet
					clone.Version = RTPPacketVersionAVC
					clone.Payload = pps
					if err = push(&clone); err != nil {
						return err
					}
				}

				clone = *packet
				clone.Version = RTPPacketVersionAVC
				clone.Payload = data
				return push(&clone)
			}
		}
	}
}

func RTPPay(mtu uint16) streamer.WrapperFunc {
	payloader := &Payloader{IsAVC: true}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version == RTPPacketVersionAVC {
				payloads := payloader.Payload(mtu, packet.Payload)
				for i, payload := range payloads {
					clone := rtp.Packet{
						Header: rtp.Header{
							Version: 2,
							Marker:  i == len(payloads)-1,
							//PayloadType:    packet.PayloadType,
							SequenceNumber: sequencer.NextSequenceNumber(),
							Timestamp:      packet.Timestamp,
							//SSRC:           packet.SSRC,
						},
						Payload: payload,
					}
					if err := push(&clone); err != nil {
						return err
					}
				}
				return nil
			}

			return push(packet)
		}
	}
}
