package wyze

import (
	"encoding/binary"
	"errors"
	"time"
)

// BuildSDIndexRequest permits only the two read queries researched for this probe.
// 10450 asks for SD path metadata with the fixed payload 03. It does not prove
// that ordinary continuous footage can be exported; the app uses it for albums.
func BuildSDIndexRequest(opcode uint16) ([]byte, error) {
	var payload []byte
	switch opcode {
	case 10450:
		payload = []byte{3}
	case 10352:
	default:
		return nil, errors.New("unsupported read query")
	}
	frame := make([]byte, 16+len(payload))
	copy(frame, "HL")
	frame[2] = 5
	binary.LittleEndian.PutUint16(frame[4:], opcode)
	binary.LittleEndian.PutUint16(frame[6:], uint16(len(payload)))
	copy(frame[16:], payload)
	return frame, nil
}

func parseSDIndexResponse(frame []byte, opcode uint16) ([]byte, error) {
	if len(frame) < 16 || frame[0] != 'H' || frame[1] != 'L' ||
		binary.LittleEndian.Uint16(frame[4:]) != opcode+1 {
		return nil, errors.New("unexpected read-query response")
	}
	length := int(binary.LittleEndian.Uint16(frame[6:]))
	if len(frame) < 16+length {
		return nil, errors.New("truncated read-query response")
	}
	return append([]byte(nil), frame[16:16+length]...), nil
}

// ReadSDIndex sends one allowlisted query over an already authenticated session.
// It never starts a producer, sets resolution, or sends any storage write command.
func (c *Client) ReadSDIndex(opcode uint16, timeout time.Duration) ([]byte, error) {
	frame, err := BuildSDIndexRequest(opcode)
	if err != nil {
		return nil, err
	}
	if c == nil || c.conn == nil {
		return nil, errors.New("authenticated connection required")
	}
	if timeout <= 0 || timeout > 30*time.Second {
		return nil, errors.New("query timeout must be within 30 seconds")
	}
	response, err := c.conn.WriteAndWaitIOCtrl(frame, c.matchHL(opcode+1), timeout)
	if err != nil {
		// Network/library errors must not expose credential-bearing URLs or bodies.
		return nil, errors.New("read query failed or timed out")
	}
	return parseSDIndexResponse(c.extractHL(response), opcode)
}
