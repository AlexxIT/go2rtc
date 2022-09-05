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
			//	"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d\n",
			//	track.Codec.Name, nalUnitType, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC,
			//)

			// NALu packets can be split in different ways:
			// - single type 7 and type 8 packets
			// - join type 7 and type 8 packet (type 24)
			// - split type 5 on multiple 28 packets
			// - split type 5 on multiple separate 28 packets
			units, err := depack.Unmarshal(packet.Payload)
			if len(units) == 0 || err != nil {
				return nil
			}

			for {
				i := int(binary.BigEndian.Uint32(units)) + 4
				unitAVC := units[:i]

				unitType := NALUType(unitAVC)
				switch unitType {
				case NALUTypeSPS:
					//println("new SPS")
					sps = unitAVC
					return nil
				case NALUTypePPS:
					//println("new PPS")
					pps = unitAVC
					return nil
				}

				// ffmpeg with `-tune zerolatency` enable option `-x264opts sliced-threads=1`
				// and every NALU will be sliced to multiple NALUs
				if !packet.Marker {
					buffer = append(buffer, unitAVC...)
					return nil
				}

				if buffer != nil {
					buffer = append(buffer, unitAVC...)
					unitAVC = buffer
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
				clone.Payload = unitAVC
				if err = push(&clone); err != nil {
					return err
				}

				if len(units) == i {
					return nil
				}

				units = units[i:]
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
