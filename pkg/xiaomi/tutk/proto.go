package tutk

import (
	"encoding/binary"
	"time"
)

func (c *Conn) WriteCh0(msg []byte) error {
	binary.LittleEndian.PutUint16(msg[6:], c.seqSendCh0)
	c.seqSendCh0++
	return c.Write(msg)
}

func (c *Conn) WriteCh1(msg []byte) error {
	binary.LittleEndian.PutUint16(msg[6:], c.seqSendCh1)
	c.seqSendCh1++
	msg[14] = 1 // channel
	return c.Write(msg)
}

func (c *Conn) msgConnectByUID(uid []byte, i byte) []byte {
	const size = 68 // or 52 or 68 or 88
	b := make([]byte, size)
	copy(b, "\x04\x02\x19\x02")
	b[4] = size - 16
	copy(b[8:], "\x01\x06\x21\x00")
	copy(b[16:], uid)
	copy(b[52:], "\x00\x03\x01\x02") // or 07000303 or 01010204
	copy(b[56:], c.sid[8:])
	b[64] = i // 1 or 2
	return b
}

func (c *Conn) msgAvClientStart() []byte {
	const size = 566 + 32
	msg := c.msg(size)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x00\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[16:], size-52)
	//cmd[18] = 1 // ???
	binary.LittleEndian.PutUint32(cmd[20:], uint32(time.Now().UnixMilli()))

	// important values for some cameras (not for df3)
	data := cmd[cmdHdrSize:]
	copy(data, "Miss")
	copy(data[257:], "client")

	// 0100000004000000fb071f000000000000000000000003000000000001000000
	cfg := msg[566:]
	cfg[0] = 0 // 0 - simple proto, 1 - complex proto with "0Cxx" commands
	cfg[4] = 4
	copy(cfg[8:], "\xfb\x07\x1f\x00")
	cfg[22] = 3
	cfg[28] = 1
	return msg
}

func (c *Conn) msg(size uint16) []byte {
	b := make([]byte, size)
	copy(b, "\x04\x02\x19\x0a")
	binary.LittleEndian.PutUint16(b[4:], size-16)
	copy(b[8:], "\x07\x04\x21\x00")
	copy(b[12:], c.sid)
	return b
}

const (
	msgPing = iota + 1
	msgClientStart00
	msgClientStart20
	msgCommand
	msgCounters
	msgMediaChunk
	msgMediaFrame
	msgMediaLost
	msgCh5
	msgUnknown0007
	msgUnknown0008
	msgUnknown0010
	msgUnknown0013
	msgUnknown0a08
	msgDafang0012
	msgDafang0071
)

// handleMsg will return parsed msg type or zero
func (c *Conn) handleMsg(msg []byte) int8 {
	//log.Printf("<- %x", msg)
	// off sample
	// 0   0402      tutk magic
	// 2   120a      tutk version (120a, 190a...)
	// 4   0800      msg size = len(b)-16
	// 6   0000      channel seq
	// 8   28041200  msg type
	// 14  0100      channel (not all msg)
	// 28  0700      msg data (not all msg)
	switch msg[8] {
	case 0x28:
		_ = c.Write(msgAckPing(msg))
		return msgPing
	case 0x08:
		switch ch := msg[14]; ch {
		case 0:
			return c.handleCh0(msg[28:])
		case 1:
			return c.handleCh1(msg[28:])
		case 5:
			return c.handleCh5(msg)
		}
	}
	return 0
}

func (c *Conn) handleCh1(cmd []byte) int8 {
	// Channel 1 used for two-way audio. It's important:
	// - answer on 0000 command with exact config response (can't set simple proto)
	// - send 0012 command at start
	// - respond on every 0008 command for smooth playback
	switch cid := string(cmd[:2]); cid {
	case "\x00\x00": // client start
		_ = c.WriteCh1(c.msgAck0000(cmd))
		_ = c.WriteCh1(c.msg0012())
		return msgClientStart00
	case "\x00\x07": // time sync without data
		_ = c.WriteCh1(c.msgAck0007(cmd))
		return msgUnknown0007
	case "\x00\x08": // time sync with data
		_ = c.WriteCh1(c.msgAck0008(cmd))
		return msgUnknown0008
	case "\x00\x13": // ack for 0012
		return msgUnknown0013
	case "\x00\x20": // client start2
		//_ = c.WriteCh1(c.msgAck0020(cmd))
		return msgClientStart20
	case "\x09\x00": // counters sync
		return msgCounters
	case "\x0a\x08": // unknown
		_ = c.WriteCh1(c.msgAck0A08(cmd))
		return msgUnknown0a08
	}
	return 0
}

func (c *Conn) handleCh5(msg []byte) int8 {
	if len(msg) != 48 {
		return 0
	}

	// <- [48] 0402190a 20000400 07042100 7ecc05000c0000007ecc93c456c2561f 5a97c2f101050000000000000000000000010000
	// -> [48] 0402190a 20000400 08041200 7ecc05000c0000007ecc93c456c2561f 5a97c2f141050000000000000000000000010000
	copy(msg[8:], "\x07\x04\x21\x00")
	msg[32] = 0x41
	_ = c.Write(msg)
	return msgCh5
}

const msgHhrSize = 28
const cmdHdrSize = 24

func (c *Conn) msgAck0000(msg28 []byte) []byte {
	// <- 000008000000000000000000000000001a0200004f47c714 ... 00000000000000000100000004000000fb071f00000000000000000000000300
	// -> 00140b00000000000000000000000000200000004f47c714     00000000000000000100000004000000fb071f00000000000000000000000300
	const cmdDataSize = 32
	msg := c.msg(msgHhrSize + cmdHdrSize + cmdDataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x14\x0b\x00")
	cmd[16] = cmdDataSize
	copy(cmd[20:], msg28[20:24]) // request id (random)

	// Important to answer with same data.
	data := cmd[cmdHdrSize:]
	copy(data, msg28[len(msg28)-32:])
	return msg
}

//func (c *Conn) msgAck0020(msg28 []byte) []byte {
//	const cmdDataSize = 36
//
//	msg := c.msg(msgHhrSize + cmdHdrSize + cmdDataSize)
//
//	cmd := msg[msgHhrSize:]
//	copy(cmd, "\x00\x14\x0b\x00")
//	cmd[16] = cmdDataSize
//	copy(cmd[20:], msg28[20:24]) // request id (random)
//
//	data := cmd[cmdHdrSize:]
//	data[5] = 1
//	data[7] = 1
//	data[8] = 1
//	data[12] = 4
//	copy(data[16:], "\xfb\x07\x1f\x00")
//	data[30] = 3
//	data[32] = 1
//	return msg
//}

func (c *Conn) msg0012() []byte {
	// -> 00120b000000000000000000000000000c00000000000000020000000100000001000000
	const dataSize = 12
	msg := c.msg(msgHhrSize + cmdHdrSize + dataSize)
	cmd := msg[msgHhrSize:]

	copy(cmd, "\x00\x12\x0b\x00")
	cmd[16] = dataSize
	data := cmd[cmdHdrSize:]

	data[0] = 2
	data[4] = 1
	data[9] = 1
	return msg
}

func (c *Conn) msgAck0007(msg28 []byte) []byte {
	// <- 000708000000000000000000000000000c00000001000000000000001c551f7a00000000
	// -> 010a0b00000000000000000000000000000000000100000000000000
	msg := c.msg(msgHhrSize + 28)
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x01\x0a\x0b\x00")
	cmd[20] = 1
	return msg
}

func (c *Conn) msgAck0008(msg28 []byte) []byte {
	// <- 000808000000000000000000000000000000f9f0010000000200000050f31f7a
	// -> 01090b0000000000000000000000000000000000010000000200000050f31f7a
	msg := c.msg(msgHhrSize + 28)
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x01\x09\x0b\x00")
	copy(cmd[20:], msg28[20:])
	return msg
}

func (c *Conn) msgAck0A08(msg28 []byte) []byte {
	// <- 0a080b005b0000000b51590002000000
	// -> 0b000b00000001000b5103000300000000000000
	msg := c.msg(msgHhrSize + 20)
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x0b\x00\x0b\x00")
	copy(cmd[8:], msg28[8:10])
	return msg
}

func msgAckPing(req []byte) []byte {
	// <- [24] 0402120a 08000000 28041200 000000005b0d4202070aa8c0
	// -> [24] 04021a0a 08000000 27042100 000000005b0d4202070aa8c0
	req[8] = 0x27
	req[10] = 0x21
	return req
}
