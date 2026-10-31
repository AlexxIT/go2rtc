package wyze

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// ProbeRDTHandshake enables the ephemeral RDT server, attempts a five-second
// channel-10 secure handshake, and always attempts to disable the server again.
// Full SHA256(ENR), rather than AV's legacy DerivePSK truncation behavior, is
// supported by the native RDT client callback in RDT_EXPERIMENTAL.md.
// Preparation/cleanup follow GalleryPage.R2/onPause and mediaEnableControl:
// K10010 mediaType=4, enable=1 / disable=2. FW6114-RDT-activation.c independently
// confirms protocol_channel_switch_handler starts/stops the RDT server for these
// values. No video/audio flag, resolution, file or RDT application payload is sent.
// Each control acknowledgement is bounded to five seconds (15 seconds overall).
func (c *Client) ProbeRDTHandshake() (result error) {
	if c == nil {
		return errors.New("authenticated connection required")
	}
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed || c.conn == nil || c.enr == "" {
		return errors.New("authenticated connection required")
	}
	// Even a lost enable acknowledgement may leave the ephemeral server active.
	defer func() {
		if err := c.setRDTEnabled(false); err != nil {
			result = errors.Join(result, fmt.Errorf("RDT disable failed: %w", err))
		}
	}()
	if err := c.setRDTEnabled(true); err != nil {
		return fmt.Errorf("RDT enable failed: %w", err)
	}
	psk := sha256.Sum256([]byte(c.enr))
	return c.conn.ProbeRDTHandshake(psk[:])
}

func (c *Client) setRDTEnabled(enabled bool) error {
	matchHL := c.matchHL(KCmdControlChannelResp)
	match := func(data []byte) bool {
		if !matchHL(data) {
			return false
		}
		frame := c.extractHL(data)
		// Ignore unrelated video/audio acknowledgements. Version, declared
		// length and result remain unfiltered for numeric diagnostics below.
		return len(frame) > 16 && frame[16] == MediaTypeRDT
	}
	response, err := c.conn.WriteAndWaitIOCtrl(c.buildK10010(MediaTypeRDT, enabled), match, 5*time.Second)
	if err != nil {
		return errors.New("no HL10011 acknowledgement within control deadline")
	}
	frame := c.extractHL(response)
	if !isRDTControlResponse(frame) || frame[17] != 1 {
		return rdtControlDiagnostic(frame)
	}
	return nil
}

// CommandTreatment passes 10011 payload[0] as media type and payload[1] as
// result; dealRES_CHANNEL_CONTROL accepts result 1 for the requested state.
func isRDTControlResponse(frame []byte) bool {
	if len(frame) < 18 || string(frame[:2]) != "HL" ||
		binary.LittleEndian.Uint16(frame[4:]) != KCmdControlChannelResp {
		return false
	}
	// The request builder uses version 5. This firmware's authenticated live
	// 10011 acknowledgements advertise device protocol version 37 instead.
	version := binary.LittleEndian.Uint16(frame[2:])
	if version != 5 && version != 37 {
		return false
	}
	length := int(binary.LittleEndian.Uint16(frame[6:]))
	return length >= 2 && len(frame) >= 16+length && frame[16] == MediaTypeRDT
}

// Diagnostic fields are strictly numeric; no raw response or upstream text.
func rdtControlDiagnostic(frame []byte) error {
	version, length, media, result := -1, -1, -1, -1
	if len(frame) >= 16 {
		version = int(binary.LittleEndian.Uint16(frame[2:]))
		length = int(binary.LittleEndian.Uint16(frame[6:]))
	}
	if len(frame) > 16 {
		media = int(frame[16])
	}
	if len(frame) > 17 {
		result = int(frame[17])
	}
	return fmt.Errorf("HL10011 rejected (frame_len=%d, version=%d, payload_len=%d, media=%d, result=%d)", len(frame), version, length, media, result)
}
