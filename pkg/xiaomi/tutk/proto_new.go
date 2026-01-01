package tutk

import (
	"encoding/binary"
	"fmt"
	"time"
)

func (c *Conn) newMsgCtrl(ctrlType uint16, ctrlData []byte) []byte {
	size := msgHhrSize + 28 + 4 + uint16(len(ctrlData))
	msg := c.msg(size)

	// 0  0070      command
	// 2  0b00      version
	// 4  1000      seq
	// 6  0076      ???
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x00\x70\x0b\x00")
	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++

	// 8  0070      command (second time)
	// 10 0300      seq
	// 12 0100      chunks count
	// 14 0000      chunk seq (starts from 0)
	// 16 5500      size
	// 18 0000      random msg id (always 0)
	// 20 03000000  seq (second time)
	// 24 00000000
	// 28 01010000  ctrlType
	cmd[9] = 0x70
	cmd[12] = 1
	binary.LittleEndian.PutUint16(cmd[16:], size-52)

	binary.LittleEndian.PutUint16(cmd[10:], c.seqSendCmd2)
	binary.LittleEndian.PutUint16(cmd[20:], c.seqSendCmd2)
	c.seqSendCmd2++

	data := cmd[28:]
	binary.LittleEndian.PutUint16(data, ctrlType)
	copy(data[4:], ctrlData)
	return msg
}

func (c *Conn) newHandlerCh0() func(msg []byte) int8 {
	var waitData []byte
	var waitSeq uint16

	return func(cmd []byte) int8 {
		switch cmd[0] {
		case 0x07, 0x05:
			flag := cmd[1]

			var cmd2 []byte
			if flag&0b1000 == 0 {
				// off sample
				// 0   0700      command
				// 2   0b00      version
				// 4   2700      seq
				// 6   0000      ???
				// 8   0700      command (second time)
				// 10  1400      seq
				// 12  1300      chunks count per this frame
				// 14  1100      chunk seq, starts from 0 (0x20 for last chunk)
				// 16  0004      frame data size
				// 18  0000      random msg id (always 0)
				// 20  02000000  previous frame seq, starts from 0
				// 24  03000000  current frame seq, starts from 1
				cmd2 = cmd[8:]
			} else {
				// off sample
				// 0   070d0b00
				// 4   30000000
				// 8   5c965500  ???
				// 12  ffff0000  ???
				// 16  0701      fixed command
				// 18  190001002000a802000006000000070000000
				cmd2 = cmd[16:]
			}

			seq := binary.LittleEndian.Uint16(cmd2[2:])

			// Check if this is first chunk for frame.
			// Handle protocol bug "0x20 chunk seq for last chunk" and sometimes
			// "0x20 chunk seq for first chunk if only one chunk".
			if binary.LittleEndian.Uint16(cmd2[6:]) == 0 || binary.LittleEndian.Uint16(cmd2[4:]) == 1 {
				waitData = waitData[:0]
				waitSeq = seq
			} else if seq != waitSeq {
				return msgMediaLost
			}

			if flag&0b0001 == 0 {
				waitData = append(waitData, cmd2[20:]...)
				waitSeq++
				return msgMediaChunk
			}

			c.seqRecvPkt1 = seq
			_ = c.WriteCh0(c.msgAckCounters())

			data := cmd2[20:]
			n := len(data) - 32
			waitData = append(waitData, data[:n]...)

			packetData := make([]byte, 32+len(waitData))
			copy(packetData, data[n:])
			copy(packetData[32:], waitData)

			select {
			case c.rawPkt <- packetData:
			default:
				c.err = fmt.Errorf("%s: media queue is full", "tutk")
				return -1
			}
			return msgMediaFrame

		case 0x00:
			_ = c.WriteCh0(c.msgAckCounters())
			c.seqRecvCmd2 = binary.LittleEndian.Uint16(cmd[2:])

			switch cmd[1] {
			case 0x10:
				return msgUnknown0010 // unknown
			case 0x70:
				select {
				case c.rawCmd <- cmd[28:]:
				default:
				}
				return msgCommand // cmd from camera
			}

		case 0x09:
			// off  sample
			// 0    09000b00  cmd1
			// 4    0d000000  seqCmd1
			// 12   0000      seqRecvCmd2
			seq := binary.LittleEndian.Uint16(cmd[12:])
			if c.seqSendCmd1 > seq {
				if c.cmdAck != nil {
					c.cmdAck()
				}
			}
			return msgCounters

		case 0x0a:
			// seq sample
			// 0   0a080b00
			// 4   03000000
			// 8   e2043200
			// 12  01000000
			_ = c.WriteCh0(c.msgAck0A08(cmd))
			return msgUnknown0a08
		}

		return 0
	}
}

func (c *Conn) msgAckCounters() []byte {
	msg := c.msg(msgHhrSize + cmdHdrSize)

	// off  sample
	// 0    09000b00  cmd1
	// 4    2700      seqCmd1
	// 6    0000
	// 8    1300      seqRecvPkt0
	// 10   2600      seqRecvPkt1
	// 12   0400      seqRecvCmd2
	// 14   00000000
	// 18   1400      seqSendCnt
	// 20   d91a      random
	// 22   0000
	cmd := msg[msgHhrSize:]
	copy(cmd, "\x09\x00\x0b\x00")

	binary.LittleEndian.PutUint16(cmd[4:], c.seqSendCmd1)
	c.seqSendCmd1++

	// seqRecvPkt0 stores previous value of seqRecvPkt1
	// don't understand why this needs
	binary.LittleEndian.PutUint16(cmd[8:], c.seqRecvPkt0)
	c.seqRecvPkt0 = c.seqRecvPkt1
	binary.LittleEndian.PutUint16(cmd[10:], c.seqRecvPkt1)
	binary.LittleEndian.PutUint16(cmd[12:], c.seqRecvCmd2)

	binary.LittleEndian.PutUint16(cmd[18:], c.seqSendCnt)
	c.seqSendCnt++
	binary.LittleEndian.PutUint16(cmd[20:], uint16(time.Now().UnixNano()))
	return msg
}
