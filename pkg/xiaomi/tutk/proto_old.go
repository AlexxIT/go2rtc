package tutk

import (
	"encoding/binary"
	"fmt"
)

func (c *Conn) oldMsgCtrl(ctrlType uint16, ctrlData []byte) []byte {
	size := msgHhrSize + cmdHdrSize + 4 + uint16(len(ctrlData))
	msg := c.msg(size)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x70\x0b\x00")

	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++

	binary.LittleEndian.PutUint16(cmd[16:], size-52)
	//binary.LittleEndian.PutUint32(cmd[20:], uint32(time.Now().UnixMilli()))

	data := cmd[cmdHdrSize:]
	binary.LittleEndian.PutUint16(data, ctrlType)
	copy(data[4:], ctrlData)
	return msg
}

func (c *Conn) oldHandlerCh0() func([]byte) int8 {
	var waitSeq uint16
	var waitSize uint32
	var waitData []byte

	return func(cmd []byte) int8 {
		// 0  01030800  command + version
		// 4  00000000  fixed
		// 8  ac880100  total size
		// 12 6200      chunk seq
		// 14 2000      ???
		// 16 cc00      size
		// 18 0000
		// 20 01000000  fixed

		switch cmd[0] {
		case 0x01:
			switch cmd[1] {
			case 0x03:
				seq := binary.LittleEndian.Uint16(cmd[12:])
				if seq != waitSeq {
					waitSeq = 0
					return msgMediaLost
				}
				if seq == 0 {
					waitData = waitData[:0]
					waitSize = binary.LittleEndian.Uint32(cmd[8:]) + 32
				}

				waitData = append(waitData, cmd[24:]...)
				if n := uint32(len(waitData)); n < waitSize {
					waitSeq++
					return msgMediaChunk
				} else if n > waitSize {
					waitSeq = 0
					return msgMediaLost
				}

				// create a buffer for the header and collected data
				packetData := make([]byte, waitSize)
				// there's a header at the end - let's move it to the beginning
				copy(packetData, waitData[waitSize-32:])
				copy(packetData[32:], waitData)

				select {
				case c.rawPkt <- packetData:
				default:
					c.err = fmt.Errorf("%s: media queue is full", "tutk")
					return -1
				}

				waitSeq = 0
				return msgMediaFrame

			case 0x04:
				waitSize2 := binary.LittleEndian.Uint32(cmd[8:])
				waitData2 := cmd[24:]

				if uint32(len(waitData2)) != waitSize2 {
					return -1 // shouldn't happened for audio
				}

				packetData := make([]byte, waitSize2)
				copy(packetData, waitData2)

				select {
				case c.rawPkt <- packetData:
				default:
					c.err = fmt.Errorf("%s: media queue is full", "tutk")
					return -1
				}
				return msgMediaFrame
			}

		case 0x00:
			switch cmd[1] {
			case 0x70:
				_ = c.WriteCh0(c.msgAck0070(cmd))
				select {
				case c.rawCmd <- cmd[24:]:
				default:
				}
				return msgCommand
			case 0x12:
				_ = c.WriteCh0(c.msgAck0012(cmd))
				return msgDafang0012
			case 0x71:
				if c.cmdAck != nil {
					c.cmdAck()
				}
				return msgDafang0071
			}
		}

		return 0
	}
}

func (c *Conn) msgAck0070(msg28 []byte) []byte {
	// <- [104] 0402120a 58000300 08041200 e6e8 0000 0c000000e6e839da66b0dc14 00700800010000000000000000000000 3400 00007625a02f ...
	// -> [ 52] 0402190a 24000400 07042100 e6e8 0000 0c000000e6e839da66b0dc14 00710800010000000000000000000000 0000 00007625a02f
	msg := c.msg(52)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x71\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++
	copy(cmd[8:], msg28[8:10])

	return msg
}

func (c *Conn) msgAck0012(msg28 []byte) []byte {
	// <- [64] 0402120a 30000000 08041200 e6e800000c000000e6e839da66b0dc14 001208000000000000000000000000000c00000000000000 020000000100000001000000
	// -> [72] 0402190a 38000300 07042100 e6e800000c000000e6e839da66b0dc14 00130b000000000000000000000000001400000000000000 0200000001000000010000000000000000000000
	const size = 72
	msg := c.msg(size)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x13\x0b\x00")
	cmd[16] = size - 52 // data size

	data := cmd[cmdHdrSize:]
	copy(data, msg28[cmdHdrSize:])

	return msg
}
