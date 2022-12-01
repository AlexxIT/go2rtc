package mjpeg

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	var header, payload []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//fmt.Printf(
			//	"[RTP] codec: %s, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, mark: %v\n",
			//	track.Codec.Name, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker,
			//)

			// https://www.rfc-editor.org/rfc/rfc2435#section-3.1
			b := packet.Payload

			// 3.1.  JPEG header
			t := b[4]

			// 3.1.7.  Restart Marker header
			if 64 <= t && t <= 127 {
				b = b[12:] // skip it
			} else {
				b = b[8:]
			}

			if header == nil {
				var lqt, cqt []byte

				// 3.1.8.  Quantization Table header
				q := packet.Payload[5]
				if q >= 128 {
					lqt = b[4:68]
					cqt = b[68:132]
					b = b[132:]
				} else {
					lqt, cqt = MakeTables(q)
				}

				// https://www.rfc-editor.org/rfc/rfc2435#section-3.1.5
				// The maximum width is 2040 pixels.
				w := uint16(packet.Payload[6]) << 3
				h := uint16(packet.Payload[7]) << 3

				// fix 2560x1920 and 2560x1440
				if w == 512 && (h == 1920 || h == 1440) {
					w = 2560
				}

				//fmt.Printf("t: %d, q: %d, w: %d, h: %d\n", t, q, w, h)
				header = MakeHeaders(t, w, h, lqt, cqt)
			}

			// 3.1.9.  JPEG Payload
			payload = append(payload, b...)

			if !packet.Marker {
				return nil
			}

			b = append(header, payload...)
			if end := b[len(b)-2:]; end[0] != 0xFF && end[1] != 0xD9 {
				b = append(b, 0xFF, 0xD9)
			}

			header = nil
			payload = nil

			packet.Payload = b
			return push(packet)
		}
	}
}
