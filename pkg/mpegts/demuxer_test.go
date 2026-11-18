package mpegts

import (
	"bytes"
	"testing"
)

// tsPacket builds one 188-byte TS packet carrying payload on pid, padded with 0xFF.
func tsPacket(pid uint16, start bool, payload []byte) []byte {
	b := make([]byte, PacketSize)
	b[0] = SyncByte
	b[1] = byte(pid>>8) & 0x1F
	if start {
		b[1] |= 0x40
	}
	b[2] = byte(pid)
	b[3] = 0x10 // payload only
	n := copy(b[4:], payload)
	for i := 4 + n; i < PacketSize; i++ {
		b[i] = 0xFF
	}
	return b
}

// section wraps a PSI body (everything after the section length) with pointer field, table id and length; the CRC
// is not checked by the demuxer, so zeros do.
func section(tableID byte, body []byte) []byte {
	length := len(body) + 4 // + CRC32
	return append(append([]byte{0x00, tableID, 0xB0 | byte(length>>8), byte(length)}, body...), 0, 0, 0, 0)
}

func pat(pmtPID uint16) []byte {
	return section(tableIDPAT, []byte{0x00, 0x01, 0xC1, 0x00, 0x00, 0x00, 0x01, 0xE0 | byte(pmtPID>>8), byte(pmtPID)})
}

func pmt(version byte, streams ...[2]uint16) []byte {
	body := []byte{0x00, 0x01, 0xC1 | version<<1, 0x00, 0x00, 0xE1, 0x00, 0xF0, 0x00}
	for _, s := range streams {
		body = append(body, byte(s[0]), 0xE0|byte(s[1]>>8), byte(s[1]), 0xF0, 0x00)
	}
	return section(tableIDPMT, body)
}

func TestDemuxerLaterPMTAddsStream(t *testing.T) {
	const pmtPID, videoPID, audioPID = 0x1000, 0x100, 0x101
	var ts bytes.Buffer
	ts.Write(tsPacket(0, true, pat(pmtPID)))
	ts.Write(tsPacket(pmtPID, true, pmt(0, [2]uint16{StreamTypeH264, videoPID})))
	ts.Write(tsPacket(pmtPID, true, pmt(1, [2]uint16{StreamTypeH264, videoPID}, [2]uint16{StreamTypePCMUTapo, audioPID})))
	// A second program's PMT re-announcing an H264 PID must not be merged (it would interleave into one receiver).
	ts.Write(tsPacket(pmtPID, true, pmt(2, [2]uint16{StreamTypeH264, 0x200})))

	d := NewDemuxer()
	for {
		if _, err := d.ReadPacket(&ts); err != nil {
			break
		}
	}

	if d.pes[audioPID] == nil || d.pes[audioPID].StreamType != StreamTypePCMUTapo {
		t.Fatalf("audio PID from the later PMT not tracked: %+v", d.pes)
	}
	if d.pes[0x200] != nil {
		t.Errorf("a second H264 PID was merged: %+v", d.pes)
	}
}

// Garbage on PSI PIDs (bad pointer, oversized length, continuation packets) must be dropped, never read past the
// packet: a panic here takes the whole process down.
func TestDemuxerMalformedPSINoPanic(t *testing.T) {
	const pmtPID = 0x1000
	var ts bytes.Buffer
	ts.Write(tsPacket(0, true, pat(pmtPID)))
	ts.Write(tsPacket(pmtPID, true, pmt(0, [2]uint16{StreamTypeH264, 0x100})))
	junk := bytes.Repeat([]byte{0xFF}, PacketSize)
	ts.Write(tsPacket(pmtPID, false, junk))                                                                 // continuation: not a section start
	ts.Write(tsPacket(pmtPID, true, append([]byte{0xB0}, junk...)))                                         // pointer field past the packet
	ts.Write(tsPacket(0, true, []byte{0x00, 0x00, 0xB3, 0xFF}))                                             // PAT section length 1023
	ts.Write(tsPacket(pmtPID, true, []byte{0x00, 0x02, 0xB0, 0x20, 0, 1, 0xC1, 0, 0, 0xE1, 0, 0xF3, 0xFF})) // huge program info

	d := NewDemuxer()
	for {
		if _, err := d.ReadPacket(&ts); err != nil {
			break
		}
	}
}

// The same garbage before any PMT, where even the stock demuxer parsed PSI unchecked.
func TestDemuxerMalformedPSIBeforePMTNoPanic(t *testing.T) {
	for i, pkt := range [][]byte{
		tsPacket(0, true, append([]byte{0xB0}, bytes.Repeat([]byte{0xFF}, PacketSize)...)), // pointer past the packet
		tsPacket(0, true, []byte{0x00, 0x00, 0xB3, 0xFF}),                                  // PAT section length 1023
		tsPacket(0, true, []byte{0x00, 0x00, 0xB0, 0xF0, 0x00, 0x01, 0xC1, 0x00, 0x00}),    // PAT length past the packet
	} {
		var ts bytes.Buffer
		ts.Write(pkt)
		d := NewDemuxer()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d: panic: %v", i, r)
				}
			}()
			for {
				if _, err := d.ReadPacket(&ts); err != nil {
					break
				}
			}
		}()
	}
}
