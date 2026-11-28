package wyze

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

const (
	KCmdGetPTZPosition  = 11006
	KCmdGetPTZPositionR = 11007
)

// PTZPosition is the camera's position in its native coordinate system.
// ControlField is firmware-defined and should be preserved without interpretation.
type PTZPosition struct {
	ControlField int32
	X            int16
	Y            uint8
}

// Position reads the camera's current pan/tilt position without moving it.
func (c *Client) Position() (PTZPosition, error) {
	if c.conn == nil {
		return PTZPosition{}, fmt.Errorf("wyze: connection is nil")
	}

	data, err := c.conn.WriteAndWaitIOCtrl(
		tutk.HL(KCmdGetPTZPosition, nil),
		c.matchHL(KCmdGetPTZPositionR),
		5*time.Second,
	)
	if err != nil {
		return PTZPosition{}, fmt.Errorf("wyze: get PTZ position: %w", err)
	}

	hlData := c.extractHL(data)
	if hlData == nil {
		return PTZPosition{}, fmt.Errorf("wyze: PTZ position response has no HL frame")
	}
	cmd, payload, ok := tutk.ParseHL(hlData)
	if !ok || cmd != KCmdGetPTZPositionR {
		return PTZPosition{}, fmt.Errorf("wyze: unexpected PTZ position response command %d", cmd)
	}

	return parsePTZPosition(payload)
}

func parsePTZPosition(payload []byte) (PTZPosition, error) {
	if len(payload) < 7 {
		return PTZPosition{}, fmt.Errorf("wyze: short PTZ position response: %d bytes", len(payload))
	}
	return PTZPosition{
		ControlField: int32(binary.LittleEndian.Uint32(payload[0:4])),
		Y:            payload[4],
		X:            int16(binary.LittleEndian.Uint16(payload[5:7])),
	}, nil
}
