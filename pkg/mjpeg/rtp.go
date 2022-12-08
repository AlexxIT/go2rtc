package mjpeg

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	buf := make([]byte, 0, 512*1024) // 512K

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			//log.Printf("[RTP] codec: %s, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, mark: %v", track.Codec.Name, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

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

			if len(buf) == 0 {
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
				buf = MakeHeaders(buf, t, w, h, lqt, cqt)
			}

			// 3.1.9.  JPEG Payload
			buf = append(buf, b...)

			if !packet.Marker {
				return nil
			}

			if end := buf[len(buf)-2:]; end[0] != 0xFF && end[1] != 0xD9 {
				buf = append(buf, 0xFF, 0xD9)
			}

			clone := *packet
			clone.Payload = buf

			buf = buf[:0] // clear buffer

			return push(&clone)
		}
	}
}

func RTPPay() streamer.WrapperFunc {
	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			return nil
		}
	}
}

//func RTPPay() streamer.WrapperFunc {
//	const packetSize = 1436
//
//	sequencer := rtp.NewRandomSequencer()
//
//	return func(push streamer.WriterFunc) streamer.WriterFunc {
//		return func(packet *rtp.Packet) error {
//			// reincode image to more common form
//			img, err := jpeg.Decode(bytes.NewReader(packet.Payload))
//			if err != nil {
//				return err
//			}
//
//			wh := img.Bounds().Size()
//			w := wh.X
//			h := wh.Y
//
//			if w > 2040 {
//				w = 2040
//			} else if w&3 > 0 {
//				w &= 3
//			}
//			if h > 2040 {
//				h = 2040
//			} else if h&3 > 0 {
//				h &= 3
//			}
//
//			if w != wh.X || h != wh.Y {
//				x0 := (wh.X - w) / 2
//				y0 := (wh.Y - h) / 2
//				rect := image.Rect(x0, y0, x0+w, y0+h)
//				img = img.(*image.YCbCr).SubImage(rect)
//			}
//
//			buf := bytes.NewBuffer(nil)
//			if err = jpeg.Encode(buf, img, nil); err != nil {
//				return err
//			}
//
//			h1 := make([]byte, 8)
//			h1[4] = 1   // Type
//			h1[5] = 255 // Q
//
//			// MBZ=0, Precision=0, Length=128
//			h2 := make([]byte, 4, 132)
//			h2[3] = 128
//
//			var jpgData []byte
//
//			p := buf.Bytes()
//
//			for jpgData == nil {
//				// 2 bytes h1
//				if p[0] != 0xFF {
//					return nil
//				}
//
//				size := binary.BigEndian.Uint16(p[2:]) + 2
//
//				// 2 bytes payload size (include 2 bytes)
//				switch p[1] {
//				case 0xD8: // 0. Start Of Image (size=0)
//					p = p[2:]
//					continue
//				case 0xDB: // 1. Define Quantization Table (size=130)
//					for i := uint16(4 + 1); i < size; i += 1 + 64 {
//						h2 = append(h2, p[i:i+64]...)
//					}
//				case 0xC0: // 2. Start Of Frame (size=15)
//					if p[4] != 8 {
//						return nil
//					}
//					h := binary.BigEndian.Uint16(p[5:])
//					w := binary.BigEndian.Uint16(p[7:])
//					h1[6] = uint8(w >> 3)
//					h1[7] = uint8(h >> 3)
//				case 0xC4: // 3. Define Huffman Table (size=416)
//				case 0xDA: // 4. Start Of Scan (size=10)
//					jpgData = p[size:]
//				}
//
//				p = p[size:]
//			}
//
//			offset := 0
//			p = make([]byte, 0)
//
//			for jpgData != nil {
//				p = p[:0]
//
//				if offset > 0 {
//					h1[1] = byte(offset >> 16)
//					h1[2] = byte(offset >> 8)
//					h1[3] = byte(offset)
//					p = append(p, h1...)
//				} else {
//					p = append(p, h1...)
//					p = append(p, h2...)
//				}
//
//				dataLen := packetSize - len(p)
//				if dataLen < len(jpgData) {
//					p = append(p, jpgData[:dataLen]...)
//					jpgData = jpgData[dataLen:]
//					offset += dataLen
//				} else {
//					p = append(p, jpgData...)
//					jpgData = nil
//				}
//
//				clone := rtp.Packet{
//					Header: rtp.Header{
//						Version:        2,
//						Marker:         jpgData == nil,
//						SequenceNumber: sequencer.NextSequenceNumber(),
//						Timestamp:      packet.Timestamp,
//					},
//					Payload: p,
//				}
//				if err := push(&clone); err != nil {
//					return err
//				}
//			}
//
//			return nil
//		}
//	}
//}
