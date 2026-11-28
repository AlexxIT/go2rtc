package wyze

import (
	"encoding/binary"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

const KCmdPTZRelative = 11000

// PTZRelative sends a relative pan/tilt movement to a Wyze Pan camera.
func (c *Client) PTZRelative(horizontal, vertical int16, speed uint8) error {
	if c.conn == nil {
		return fmt.Errorf("wyze: connection is nil")
	}

	return c.conn.WriteIOCtrl(buildPTZRelative(horizontal, vertical, speed))
}

func buildPTZRelative(horizontal, vertical int16, speed uint8) []byte {
	payload := make([]byte, 5)
	binary.LittleEndian.PutUint16(payload[0:2], uint16(horizontal))
	binary.LittleEndian.PutUint16(payload[2:4], uint16(vertical))
	payload[4] = speed
	return tutk.HL(KCmdPTZRelative, payload)
}
