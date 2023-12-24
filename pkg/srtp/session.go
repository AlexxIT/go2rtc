package srtp

import (
	"net"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

type Session struct {
	Local  *Endpoint
	Remote *Endpoint

	OnReadRTP func(packet *rtp.Packet)

	Recv int // bytes recv
	Send int // bytes send

	conn net.PacketConn // local conn endpoint

	PayloadType  uint8
	RTCPInterval time.Duration

	senderRTCP rtcp.SenderReport
	senderTime time.Time

	AudioFrameDuration  uint8
	firstTimestamp      uint32
	frameBuffer         []byte
	frameSize           []int
	audioSequenceNumber uint16
	audioPacketCount    uint32
}

type Endpoint struct {
	Addr       string
	Port       uint16
	MasterKey  []byte
	MasterSalt []byte
	SSRC       uint32

	addr net.Addr
	srtp *srtp.Context
}

func (e *Endpoint) init() (err error) {
	e.addr = &net.UDPAddr{IP: net.ParseIP(e.Addr), Port: int(e.Port)}
	e.srtp, err = srtp.CreateContext(e.MasterKey, e.MasterSalt, profile(e.MasterKey))
	return
}

func profile(key []byte) srtp.ProtectionProfile {
	switch len(key) {
	case 16:
		return srtp.ProtectionProfileAes128CmHmacSha1_80
		//case 32:
		//	return srtp.ProtectionProfileAes256CmHmacSha1_80
	}
	return 0
}

func (s *Session) init() error {
	if err := s.Local.init(); err != nil {
		return err
	}
	if err := s.Remote.init(); err != nil {
		return err
	}

	s.senderRTCP.SSRC = s.Local.SSRC
	s.senderTime = time.Now().Add(s.RTCPInterval)

	return nil
}

func (s *Session) repacketizeOpus(packet *rtp.Packet) *rtp.Packet {
	if s.firstTimestamp == 0 {
		s.firstTimestamp = packet.Header.Timestamp
	}

	framesPerPacket := s.AudioFrameDuration / 20
	toc := packet.Payload[0]
	config := (toc & 0b11111000) >> 3
	switch config {
	case SILK_NB_20, SILK_MB_20, SILK_WB_20, HYBRID_SWB_20, HYBRID_FB_20, CELT_NB_20, CELT_WB_20, CELT_SWB_20, CELT_FB_20: // 20ms
		break
	default: // frame sizes not 20ms, return packet as is
		return packet
	}

	code := toc & 0b00000011

	// ffmpeg sends 20ms Opus frames, and 60ms frame is requested (Cellular connection)
	if code == 0 && framesPerPacket == 3 {
		// merge 3 frames into one packet
		if len(packet.Payload) == 1 || len(packet.Payload) > MAX_PAYLOAD_LENGTH {
			// no frame or exceed encode size, return packet as is
			return packet
		}
		if len(s.frameSize) < 3 {
			s.frameBuffer = append(s.frameBuffer, packet.Payload[1:]...)
			s.frameSize = append(s.frameSize, len(packet.Payload[1:]))

		}
		if len(s.frameSize) == 3 {
			toc |= 0b00000011                  // code 3: signaled number of frames
			frameCountByte := byte(0b10000011) // vbr, no padding, 3 frames
			frameLengthsBytes := make([]byte, 0)
			for _, size := range s.frameSize[:2] {
				// encode size
				if size < 252 {
					frameLengthsBytes = append(frameLengthsBytes, uint8(size))
				} else {
					sizeFirstByte := 252 + (size & 0x3)
					frameLengthsBytes = append(frameLengthsBytes, uint8(sizeFirstByte), uint8((size-sizeFirstByte)>>2))
				}
			}
			packet.Payload = []byte{toc, frameCountByte}
			packet.Payload = append(packet.Payload, frameLengthsBytes...)
			packet.Payload = append(packet.Payload, s.frameBuffer...)

			if s.audioSequenceNumber == 0 {
				s.audioSequenceNumber = packet.Header.SequenceNumber
			}
			packet.Header.SequenceNumber = s.audioSequenceNumber
			s.audioSequenceNumber++

			s.frameBuffer = s.frameBuffer[:0]
			s.frameSize = s.frameSize[:0]
		} else {
			return nil
		}
	}

	// Timestamp increment = frame duration(ms) * sampling rate(kHz)
	packet.Header.Timestamp = s.firstTimestamp + s.audioPacketCount*uint32(s.AudioFrameDuration)*SAMPLE_RATE
	s.audioPacketCount++
	return packet
}

func (s *Session) WriteRTP(packet *rtp.Packet, isOpus bool) (int, error) {
	if s.Local.srtp == nil {
		return 0, nil // before init call
	}

	if now := time.Now(); now.After(s.senderTime) {
		s.senderRTCP.NTPTime = uint64(now.UnixNano())
		s.senderTime = now.Add(s.RTCPInterval)
		_, _ = s.WriteRTCP(&s.senderRTCP)
	}

	if isOpus {
		packet = s.repacketizeOpus(packet)
		if packet == nil {
			return 0, nil
		}
	}

	clone := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         packet.Marker,
			PayloadType:    s.PayloadType,
			SequenceNumber: packet.SequenceNumber,
			Timestamp:      packet.Timestamp,
			SSRC:           s.Local.SSRC,
		},
		Payload: packet.Payload,
	}

	b, err := clone.Marshal()
	if err != nil {
		return 0, err
	}

	s.senderRTCP.PacketCount++
	s.senderRTCP.RTPTime = clone.Timestamp
	s.senderRTCP.OctetCount += uint32(len(clone.Payload))

	if b, err = s.Local.srtp.EncryptRTP(nil, b, nil); err != nil {
		return 0, err
	}

	return s.conn.WriteTo(b, s.Remote.addr)
}

func (s *Session) WriteRTCP(packet rtcp.Packet) (int, error) {
	b, err := packet.Marshal()
	if err != nil {
		return 0, err
	}
	b, err = s.Local.srtp.EncryptRTCP(nil, b, nil)
	if err != nil {
		return 0, err
	}
	return s.conn.WriteTo(b, s.Remote.addr)
}

func (s *Session) ReadRTP(b []byte) {
	packet := &rtp.Packet{}

	b, err := s.Remote.srtp.DecryptRTP(nil, b, &packet.Header)
	if err != nil {
		return
	}

	if err = packet.Unmarshal(b); err != nil {
		return
	}

	if s.OnReadRTP != nil {
		s.OnReadRTP(packet)
	}
}

func (s *Session) ReadRTCP(b []byte) {
	header := rtcp.Header{}
	b, err := s.Remote.srtp.DecryptRTCP(nil, b, &header)
	if err != nil {
		return
	}

	//packets, err := rtcp.Unmarshal(b)
	//if err != nil {
	//	return
	//}
	//if report, ok := packets[0].(*rtcp.SenderReport); ok {
	//	log.Printf("[srtp] rtcp type=%d report=%v", header.Type, report)
	//}

	if header.Type != rtcp.TypeSenderReport {
		return
	}

	receiverRTCP := rtcp.ReceiverReport{SSRC: s.Local.SSRC}
	_, _ = s.WriteRTCP(&receiverRTCP)
}
