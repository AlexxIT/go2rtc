package tutk

import (
	"encoding/binary"
	"time"
)

func GenSessionID() []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(time.Now().UnixNano()))
	return b
}

func ICAM(cmd uint32, args ...byte) []byte {
	// 0   4943414d  ICAM
	// 4   d807ff00  command
	// 8   00000000000000
	// 15  02        args count
	// 16  00000000000000
	// 23  0101      args
	n := byte(len(args))
	b := make([]byte, 23+n)
	copy(b, "ICAM")
	binary.LittleEndian.PutUint32(b[4:], cmd)
	b[15] = n
	copy(b[23:], args)
	return b
}

func HL(cmdID uint16, payload []byte) []byte {
	// 0-1   "HL"       magic
	// 2     version    (typically 5)
	// 3     reserved
	// 4-5   cmdID      command ID (uint16 LE)
	// 6-7   payloadLen payload length (uint16 LE)
	// 8-15  reserved
	// 16+   payload
	const headerSize = 16
	const version = 5

	b := make([]byte, headerSize+len(payload))
	copy(b, "HL")
	b[2] = version
	binary.LittleEndian.PutUint16(b[4:], cmdID)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(payload)))
	copy(b[headerSize:], payload)
	return b
}

func ParseHL(data []byte) (cmdID uint16, payload []byte, ok bool) {
	if len(data) < 16 || data[0] != 'H' || data[1] != 'L' {
		return 0, nil, false
	}
	cmdID = binary.LittleEndian.Uint16(data[4:])
	payloadLen := binary.LittleEndian.Uint16(data[6:])
	if len(data) >= 16+int(payloadLen) {
		payload = data[16 : 16+payloadLen]
	} else if len(data) > 16 {
		payload = data[16:]
	}
	return cmdID, payload, true
}
