package tutk

import (
	"encoding/binary"
	"fmt"
	"time"
)

func (c *Conn) oldMsgCtrl(ctrlType uint16, ctrlData []byte) []byte {
	dataSize := 4 + uint16(len(ctrlData))
	msg := c.msg(msgHhrSize + cmdHdrSize + dataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x70\x0b\x00")

	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++

	binary.LittleEndian.PutUint16(cmd[16:], dataSize)
	//binary.LittleEndian.PutUint32(cmd[20:], uint32(time.Now().UnixMilli()))

	data := cmd[cmdHdrSize:]
	binary.LittleEndian.PutUint16(data, ctrlType)
	copy(data[4:], ctrlData)
	return msg
}

const pktHdrSize = 32

func (c *Conn) oldMsgAud(pkt []byte) []byte {
	// -> 01030b001d0000008802000000002800b0020bf501000000 ... 4f4455412000000088020000030400001d000000000000000bf51f7a9b0100000000000000000000
	hdr := pkt[:pktHdrSize]
	payload := pkt[pktHdrSize:]

	n := uint16(len(payload))
	dataSize := n + 8 + 32
	msg := c.msg(msgHhrSize + cmdHdrSize + dataSize)

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
	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendAud)
	c.seqSendAud++
	binary.LittleEndian.PutUint16(cmd[8:], n)
	cmd[14] = 0x28 // important!
	binary.LittleEndian.PutUint16(cmd[16:], dataSize)
	binary.LittleEndian.PutUint16(cmd[18:], uint16(time.Now().UnixMilli()))
	cmd[20] = 1

	data := cmd[cmdHdrSize:]
	copy(data, payload)
	copy(data[n:], "ODUA\x20\x00\x00\x00")
	copy(data[n+8:], hdr)

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
		// 14 2000      tail (pkt header) size?
		// 16 cc00      size
		// 18 0000
		// 20 01000000  fixed

		switch cmd[0] {
		case 0x01:
			var packetData []byte

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

				waitSeq = 0

				// create a buffer for the header and collected data
				packetData = make([]byte, waitSize)
				// there's a header at the end - let's move it to the beginning
				copy(packetData, waitData[waitSize-32:])
				copy(packetData[32:], waitData)

			case 0x04:
				// This is audio from miss audio start command. MiHome not using miss commands.
				waitSize2 := binary.LittleEndian.Uint32(cmd[8:])
				waitData2 := cmd[24:]

				if uint32(len(waitData2)) != waitSize2 {
					return -1 // shouldn't happen for audio
				}

				packetData = make([]byte, waitSize2)
				copy(packetData, waitData2)

			default:
				return 0
			}

			// fix Dafang bug (timestamp in seconds)
			binary.LittleEndian.PutUint64(packetData[16:], uint64(time.Now().UnixMilli()))

			select {
			case c.rawPkt <- packetData:
			default:
				c.err = fmt.Errorf("%s: media queue is full", "tutk")
				return -1
			}
			return msgMediaFrame

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
	// <- 00700800010000000000000000000000340000007625a02f ...
	// -> 00710800010000000000000000000000000000007625a02f
	msg := c.msg(msgHhrSize + cmdHdrSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x71\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++
	copy(cmd[8:], msg28[8:10])

	return msg
}

func (c *Conn) msgAck0012(msg28 []byte) []byte {
	// <- 001208000000000000000000000000000c00000000000000 020000000100000001000000
	// -> 00130b000000000000000000000000001400000000000000 0200000001000000010000000000000000000000
	const dataSize = 20
	msg := c.msg(msgHhrSize + cmdHdrSize + dataSize)

	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x13\x0b\x00")
	cmd[16] = dataSize

	data := cmd[cmdHdrSize:]
	copy(data, msg28[cmdHdrSize:])

	return msg
}
