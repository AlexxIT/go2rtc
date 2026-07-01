package tutk

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"time"
)

type Session interface {
	Close() error

	ClientStart(i byte, username, password string) []byte

	SendIOCtrl(ctrlType uint32, ctrlData []byte) []byte
	SendFrameData(frameInfo, frameData []byte) []byte

	RecvIOCtrl() (ctrlType uint32, ctrlData []byte, err error)
	RecvFrameData() (frameInfo, frameData []byte, err error)

	SessionRead(chID byte, buf []byte) int
	SessionWrite(chID byte, buf []byte) error
}

func NewSession16(conn net.Conn, sid8 []byte) *Session16 {
	sid16 := make([]byte, 16)
	copy(sid16[8:], sid8)
	copy(sid16, sid8[:2])
	sid16[4] = 0x0c

	return &Session16{
		conn:   conn,
		sid16:  sid16,
		rawCmd: make(chan []byte, 10),
		rawPkt: make(chan [2][]byte, 100),
	}
}

type Session16 struct {
	conn  net.Conn
	sid16 []byte

	rawCmd chan []byte
	rawPkt chan [2][]byte

	seqSendCh0 uint16
	seqSendCh1 uint16

	seqSendCmd1 uint16
	seqSendAud  uint16

	waitFSeq uint16
	waitCSeq uint16
	waitSize int
	waitData []byte
}

func (s *Session16) Close() error {
	close(s.rawCmd)
	close(s.rawPkt)
	return nil
}

func (s *Session16) Msg(size uint16) []byte {
	b := make([]byte, size)
	copy(b, magic)
	b[3] = 0x0a // connected stage
	binary.LittleEndian.PutUint16(b[4:], size-16)
	copy(b[8:], "\x07\x04\x21") // client request
	copy(b[12:], s.sid16)
	return b
}

const (
	msgHhrSize = 28
	cmdHdrSize = 24
)

func (s *Session16) ClientStart(i byte, username, password string) []byte {
	const size = 566 + 32
	msg := s.Msg(size)

	// 0    00000b0000000000000000000000000022020000fcfc7284
	// 24   4d69737300000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000
	// 281  636c69656e740000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000
	// 538  0100000004000000fb071f000000000000000000000003000000000001000000
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x00\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[16:], size-52)
	if i == 0 {
		cmd[18] = 1
	} else {
		cmd[1] = 0x20
	}
	binary.LittleEndian.PutUint32(cmd[20:], uint32(time.Now().UnixMilli()))

	// important values for some cameras (not for df3)
	data := cmd[cmdHdrSize:]
	copy(data, username)
	copy(data[257:], password)

	// 0100000004000000fb071f000000000000000000000003000000000001000000
	cfg := data[257+257:]
	//cfg[0] = 1 // 0 - simple proto, 1 - complex proto with "0Cxx" commands
	cfg[4] = 4
	copy(cfg[8:], "\xfb\x07\x1f\x00")
	cfg[22] = 3
	//cfg[28] = 1 // unknown
	return msg
}

func (s *Session16) SendIOCtrl(ctrlType uint32, ctrlData []byte) []byte {
	dataSize := 4 + uint16(len(ctrlData))
	msg := s.Msg(msgHhrSize + cmdHdrSize + dataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x70\x0b\x00")

	s.seqSendCmd1++ // start from 1, important!
	binary.LittleEndian.PutUint16(cmd[4:], s.seqSendCmd1)

	binary.LittleEndian.PutUint16(cmd[16:], dataSize)
	binary.LittleEndian.PutUint32(cmd[20:], uint32(time.Now().UnixMilli()))

	data := cmd[cmdHdrSize:]
	binary.LittleEndian.PutUint32(data, ctrlType)
	copy(data[4:], ctrlData)
	return msg
}

func (s *Session16) SendFrameData(frameInfo, frameData []byte) []byte {
	// -> 01030b001d0000008802000000002800b0020bf501000000 ... 4f4455412000000088020000030400001d000000000000000bf51f7a9b0100000000000000000000

	n := uint16(len(frameData))
	dataSize := n + 8 + 32
	msg := s.Msg(msgHhrSize + cmdHdrSize + dataSize)

	// 0   01030b00  command + version
	// 4   1d000000  seq
	// 8   8802      media size (648)
	// 10  00000000
	// 14  2800      tail (pkt header) size?
	// 16  b002      size (648 + 8 + 32)
	// 18  0bf5      random msg id (unixms)
	// 20  01000000  fixed
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x01\x03\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[4:], s.seqSendAud)
	s.seqSendAud++
	binary.LittleEndian.PutUint16(cmd[8:], n)
	cmd[14] = 0x28 // important!
	binary.LittleEndian.PutUint16(cmd[16:], dataSize)
	binary.LittleEndian.PutUint16(cmd[18:], uint16(time.Now().UnixMilli()))
	cmd[20] = 1

	data := cmd[cmdHdrSize:]
	copy(data, frameData)
	copy(data[n:], "ODUA\x20\x00\x00\x00")
	copy(data[n+8:], frameInfo)

	return msg
}

func (s *Session16) RecvIOCtrl() (ctrlType uint32, ctrlData []byte, err error) {
	buf, ok := <-s.rawCmd
	if !ok {
		return 0, nil, io.EOF
	}
	return binary.LittleEndian.Uint32(buf), buf[4:], nil
}

func (s *Session16) RecvFrameData() (frameInfo, frameData []byte, err error) {
	buf, ok := <-s.rawPkt
	if !ok {
		return nil, nil, io.EOF
	}
	return buf[0], buf[1], nil
}

func (s *Session16) SessionRead(chID byte, cmd []byte) int {
	if chID != 0 {
		return s.handleCh1(cmd)
	}

	// 0  01030800  command + version
	// 4  00000000  frame seq
	// 8  ac880100  total size
	// 12 6200      chunk seq
	// 14 2000      tail (pkt header) size
	// 16 cc00      size
	// 18 0000
	// 20 01000000  fixed

	switch cmd[0] {
	case 0x01:
		var packetData [2][]byte

		switch cmd[1] {
		case 0x03:
			frameSeq := binary.LittleEndian.Uint16(cmd[4:])
			chunkSeq := binary.LittleEndian.Uint16(cmd[12:])
			if chunkSeq == 0 {
				s.waitFSeq = frameSeq
				s.waitCSeq = 0
				s.waitData = s.waitData[:0]
				payloadSize := binary.LittleEndian.Uint32(cmd[8:])
				hdrSize := binary.LittleEndian.Uint16(cmd[14:])
				s.waitSize = int(hdrSize) + int(payloadSize)
			} else if frameSeq != s.waitFSeq || chunkSeq != s.waitCSeq {
				s.waitCSeq = 0
				return msgMediaLost
			}

			s.waitData = append(s.waitData, cmd[24:]...)
			if n := len(s.waitData); n < s.waitSize {
				s.waitCSeq++
				return msgMediaChunk
			}

			s.waitCSeq = 0

			payloadSize := binary.LittleEndian.Uint32(cmd[8:])
			packetData[0] = bytes.Clone(s.waitData[payloadSize:])
			packetData[1] = bytes.Clone(s.waitData[:payloadSize])

		case 0x04:
			data := cmd[24:]
			hdrSize := binary.LittleEndian.Uint16(cmd[14:])
			packetData[0] = bytes.Clone(data[:hdrSize])
			packetData[1] = bytes.Clone(data[hdrSize:])

		default:
			return msgUnknown
		}

		select {
		case s.rawPkt <- packetData:
		default:
			return msgError
		}
		return msgMediaFrame

	case 0x00:
		switch cmd[1] {
		case 0x70:
			_ = s.SessionWrite(0, s.msgAck0070(cmd))
			select {
			case s.rawCmd <- append([]byte{}, cmd[24:]...):
			default:
			}

			return msgCommand
		case 0x12:
			_ = s.SessionWrite(0, s.msgAck0012(cmd))
			return msgDafang0012
		case 0x71:
			return msgCommandAck
		}
	}

	return msgUnknown
}

func (s *Session16) msgAck0070(msg28 []byte) []byte {
	// <- 00700800010000000000000000000000340000007625a02f ...
	// -> 00710800010000000000000000000000000000007625a02f
	msg := s.Msg(msgHhrSize + cmdHdrSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x71")
	copy(cmd[2:], msg28[2:6])    // same version and seq
	copy(cmd[20:], msg28[20:24]) // same msg random

	return msg
}

func (s *Session16) msgAck0012(msg28 []byte) []byte {
	// <- 001208000000000000000000000000000c00000000000000 020000000100000001000000
	// -> 00130b000000000000000000000000001400000000000000 0200000001000000010000000000000000000000
	const dataSize = 20
	msg := s.Msg(msgHhrSize + cmdHdrSize + dataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x13\x0b\x00")
	cmd[16] = dataSize

	data := cmd[cmdHdrSize:]
	copy(data, msg28[cmdHdrSize:])

	return msg
}

func (s *Session16) handleCh1(cmd []byte) int {
	// Channel 1 used for two-way audio. It's important:
	// - answer on 0000 command with exact config response (can't set simple proto)
	// - send 0012 command at start
	// - respond on every 0008 command for smooth playback
	switch cid := string(cmd[:2]); cid {
	case "\x00\x00": // client start
		_ = s.SessionWrite(1, s.msgAck0000(cmd))
		_ = s.SessionWrite(1, s.msg0012())
		return msgClientStart
	case "\x00\x07": // time sync without data
		_ = s.SessionWrite(1, s.msgAck0007(cmd))
		return msgUnknown0007
	case "\x00\x08": // time sync with data
		_ = s.SessionWrite(1, s.msgAck0008(cmd))
		return msgUnknown0008
	case "\x00\x13": // ack for 0012
		return msgUnknown0013
	}
	return msgUnknown
}

func (s *Session16) msgAck0000(msg28 []byte) []byte {
	// <- 000008000000000000000000000000001a0200004f47c714 ... 00000000000000000100000004000000fb071f00000000000000000000000300
	// -> 00140b00000000000000000000000000200000004f47c714     00000000000000000100000004000000fb071f00000000000000000000000300
	const cmdDataSize = 32
	msg := s.Msg(msgHhrSize + cmdHdrSize + cmdDataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x14\x0b\x00")
	cmd[16] = cmdDataSize
	copy(cmd[20:], msg28[20:24]) // request id (random)

	// Important to answer with same data.
	data := cmd[cmdHdrSize:]
	copy(data, msg28[len(msg28)-32:])
	return msg
}

func (s *Session16) msg0012() []byte {
	// -> 00120b000000000000000000000000000c00000000000000020000000100000001000000
	const dataSize = 12
	msg := s.Msg(msgHhrSize + cmdHdrSize + dataSize)
	cmd := msg[msgHhrSize:]

	copy(cmd, "\x00\x12\x0b\x00")
	cmd[16] = dataSize
	data := cmd[cmdHdrSize:]

	data[0] = 2
	data[4] = 1
	data[9] = 1
	return msg
}

func (s *Session16) msgAck0007(msg28 []byte) []byte {
	// <- 000708000000000000000000000000000c00000001000000000000001c551f7a00000000
	// -> 010a0b00000000000000000000000000000000000100000000000000
	msg := s.Msg(msgHhrSize + 28)
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x01\x0a\x0b\x00")
	cmd[20] = 1
	return msg
}

func (s *Session16) msgAck0008(msg28 []byte) []byte {
	// <- 000808000000000000000000000000000000f9f0010000000200000050f31f7a
	// -> 01090b0000000000000000000000000000000000010000000200000050f31f7a
	msg := s.Msg(msgHhrSize + 28)
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x01\x09\x0b\x00")
	copy(cmd[20:], msg28[20:])
	return msg
}

func (s *Session16) SessionWrite(chID byte, buf []byte) error {
	switch chID {
	case 0:
		binary.LittleEndian.PutUint16(buf[6:], s.seqSendCh0)
		s.seqSendCh0++
	case 1:
		binary.LittleEndian.PutUint16(buf[6:], s.seqSendCh1)
		s.seqSendCh1++
		buf[14] = 1 // channel
	}
	_, err := s.conn.Write(buf)
	return err
}
