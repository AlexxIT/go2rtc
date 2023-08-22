package mpegts

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
)

type Muxer struct {
	pes map[uint16]*PES
}

func NewMuxer() *Muxer {
	return &Muxer{
		pes: map[uint16]*PES{},
	}
}

func (m *Muxer) AddTrack(streamType byte) (pid uint16) {
	pes := &PES{StreamType: streamType}

	// Audio streams (0xC0-0xDF), Video streams (0xE0-0xEF)
	switch streamType {
	case StreamTypeH264, StreamTypeH265:
		pes.StreamID = 0xE0
	case StreamTypeAAC, StreamTypePCMATapo:
		pes.StreamID = 0xC0
	}

	pid = pes0PID + uint16(len(m.pes))
	m.pes[pid] = pes

	return
}

func (m *Muxer) GetHeader() []byte {
	bw := bits.NewWriter(nil)
	m.writePAT(bw)
	m.writePMT(bw)
	return bw.Bytes()
}

// GetPayload - safe to run concurently with different pid
func (m *Muxer) GetPayload(pid uint16, timestamp uint32, payload []byte) []byte {
	pes := m.pes[pid]

	switch pes.StreamType {
	case StreamTypeH264, StreamTypeH265:
		payload = annexb.DecodeAVCCWithAUD(payload)
	}

	if pes.Timestamp != 0 {
		pes.PTS += timestamp - pes.Timestamp
	}
	pes.Timestamp = timestamp

	// min header size (3 byte) + adv header size (PES)
	size := 3 + 5 + len(payload)

	b := make([]byte, 6+3+5)

	b[0], b[1], b[2] = 0, 0, 1 // Packet start code prefix
	b[3] = pes.StreamID        // Stream ID

	// PES Packet length (zero value OK for video)
	if size <= 0xFFFF {
		binary.BigEndian.PutUint16(b[4:], uint16(size))
	}

	// Optional PES header:
	b[6] = 0x80 // Marker bits (binary)
	b[7] = 0x80 // PTS indicator
	b[8] = 5    // PES header length

	WriteTime(b[9:], pes.PTS)

	pes.Payload = append(b, payload...)
	pes.Size = 1 // set PUSI in first PES

	if pes.wr == nil {
		pes.wr = bits.NewWriter(nil)
	} else {
		pes.wr.Reset()
	}

	for len(pes.Payload) > 0 {
		m.writePES(pes.wr, pid, pes)
		pes.Sequence++
		pes.Size = 0
	}

	return pes.wr.Bytes()
}

const patPID = 0
const pmtPID = 0x1000
const pes0PID = 0x100

func (m *Muxer) writePAT(wr *bits.Writer) {
	m.writeHeader(wr, patPID)
	i := wr.Len() + 1 // start for CRC32
	m.writePSIHeader(wr, 0, 4)

	wr.WriteUint16(1)          // Program num
	wr.WriteBits8(0b111, 3)    // Reserved bits (all to 1)
	wr.WriteBits16(pmtPID, 13) // Program map PID

	crc := checksum(wr.Bytes()[i:])
	wr.WriteBytes(byte(crc), byte(crc>>8), byte(crc>>16), byte(crc>>24)) // CRC32 (little endian)

	m.WriteTail(wr)
}

func (m *Muxer) writePMT(wr *bits.Writer) {
	m.writeHeader(wr, pmtPID)
	i := wr.Len() + 1                               // start for CRC32
	m.writePSIHeader(wr, 2, 4+uint16(len(m.pes))*5) // 4 bytes below + 5 bytes each PES

	wr.WriteBits8(0b111, 3)    // Reserved bits (all to 1)
	wr.WriteBits16(0x1FFF, 13) // Program map PID (not used)

	wr.WriteBits8(0b1111, 4) // Reserved bits (all to 1)
	wr.WriteBits8(0, 2)      // Program info length unused bits (all to 0)
	wr.WriteBits16(0, 10)    // Program info length

	for pid := uint16(pes0PID); ; pid++ {
		pes, ok := m.pes[pid]
		if !ok {
			break
		}
		wr.WriteByte(pes.StreamType) // Stream type
		wr.WriteBits8(0b111, 3)      // Reserved bits (all to 1)
		wr.WriteBits16(pid, 13)      // Elementary PID
		wr.WriteBits8(0b1111, 4)     // Reserved bits (all to 1)
		wr.WriteBits(0, 2)           // ES Info length unused bits
		wr.WriteBits16(0, 10)        // ES Info length
	}

	crc := checksum(wr.Bytes()[i:])
	wr.WriteBytes(byte(crc), byte(crc>>8), byte(crc>>16), byte(crc>>24)) // CRC32 (little endian)

	m.WriteTail(wr)
}

func (m *Muxer) writePES(wr *bits.Writer, pid uint16, pes *PES) {
	const flagPUSI = 0b01000000_00000000
	const flagAdaptation = 0b00100000
	const flagPayload = 0b00010000

	wr.WriteByte(SyncByte)

	if pes.Size != 0 {
		pid |= flagPUSI // Payload unit start indicator (PUSI)
	}

	wr.WriteUint16(pid)

	counter := byte(pes.Sequence) & 0xF

	if size := len(pes.Payload); size < PacketSize-4 {
		wr.WriteByte(flagAdaptation | flagPayload | counter) // adaptation + payload

		// for 183 payload will be zero
		adSize := PacketSize - 4 - 1 - byte(size)
		wr.WriteByte(adSize)
		wr.WriteBytes(make([]byte, adSize)...)

		wr.WriteBytes(pes.Payload...)
		pes.Payload = nil
	} else {
		wr.WriteByte(flagPayload | counter) // only payload

		wr.WriteBytes(pes.Payload[:PacketSize-4]...)
		pes.Payload = pes.Payload[PacketSize-4:]
	}
}

func (m *Muxer) writeHeader(wr *bits.Writer, pid uint16) {
	wr.WriteByte(SyncByte)

	wr.WriteBit(0)          // Transport error indicator (TEI)
	wr.WriteBit(1)          // Payload unit start indicator (PUSI)
	wr.WriteBit(0)          // Transport priority
	wr.WriteBits16(pid, 13) // PID

	wr.WriteBits8(0, 2) // Transport scrambling control (TSC)
	wr.WriteBit(0)      // Adaptation field
	wr.WriteBit(1)      // Payload
	wr.WriteBits8(0, 4) // Continuity counter
}

func (m *Muxer) writePSIHeader(wr *bits.Writer, tableID byte, size uint16) {
	wr.WriteByte(0) // Pointer field

	wr.WriteByte(tableID) // Table ID

	wr.WriteBit(1)               // Section syntax indicator
	wr.WriteBit(0)               // Private bit
	wr.WriteBits8(0b11, 2)       // Reserved bits (all to 1)
	wr.WriteBits8(0, 2)          // Section length unused bits (all to 0)
	wr.WriteBits16(5+size+4, 10) // Section length (5 bytes below + content + 4 bytes CRC32)

	wr.WriteUint16(1)      // Table ID extension
	wr.WriteBits8(0b11, 2) // Reserved bits (all to 1)
	wr.WriteBits8(0, 5)    // Version number
	wr.WriteBit(1)         // Current/next indicator

	wr.WriteByte(0) // Section number
	wr.WriteByte(0) // Last section number
}

func (m *Muxer) WriteTail(wr *bits.Writer) {
	size := PacketSize - wr.Len()%PacketSize
	wr.WriteBytes(make([]byte, size)...)
}

func WriteTime(b []byte, t uint32) {
	_ = b[4] // bounds
	const onlyPTS = 0x20
	b[0] = onlyPTS | byte(t>>(32-3)) | 1
	b[1] = byte(t >> (24 - 2))
	b[2] = byte(t>>(16-2)) | 1
	b[3] = byte(t >> (8 - 1))
	b[4] = byte(t<<1) | 1 // t>>(0-1)
}
