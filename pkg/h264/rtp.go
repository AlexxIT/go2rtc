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
	sps = EncodeAVC(sps)
	pps = EncodeAVC(pps)

	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//println(packet.SequenceNumber, packet.Payload[0]&0x1F, packet.Payload[0], packet.Payload[1], packet.Marker, packet.Timestamp)

			data, err := depack.Unmarshal(packet.Payload)
			if len(data) == 0 || err != nil {
				return nil
			}

			naluType := NALUType(data)
			//println(naluType, len(data))

			switch naluType {
			case NALUTypeSPS:
				//println("new SPS")
				sps = data
				return nil
			case NALUTypePPS:
				//println("new PPS")
				pps = data
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

			if naluType == NALUTypeIFrame {
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
