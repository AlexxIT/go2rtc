package h264

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAVC = 0

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	depack := &codecs.H264Packet{IsAVC: true}

	sps, pps := GetParameterSet(track.Codec.FmtpLine)
	ps := EncodeAVC(sps, pps)

	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//nalUnitType := packet.Payload[0] & 0x1F
			//fmt.Printf(
			//	"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d\n",
			//	track.Codec.Name, nalUnitType, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC, packet.SequenceNumber,
			//)

			payload, err := depack.Unmarshal(packet.Payload)
			if len(payload) == 0 || err != nil {
				return nil
			}

			// ffmpeg with `-tune zerolatency` enable option `-x264opts sliced-threads=1`
			// and every NALU will be sliced to multiple NALUs
			if !packet.Marker {
				buffer = append(buffer, payload...)
				return nil
			}

			if buffer != nil {
				payload = append(buffer, payload...)
				buffer = nil
			}

			switch NALUType(payload) {
			case NALUTypeIFrame:
				payload = Join(ps, payload)
			}

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
			if packet.Version == RTPPacketVersionAVC {
				payloads := payloader.Payload(mtu, packet.Payload)
				last := len(payloads) - 1
				for i, payload := range payloads {
					clone := rtp.Packet{
						Header: rtp.Header{
							Version: 2,
							Marker:  i == last,
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
