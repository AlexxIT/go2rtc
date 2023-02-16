package mpegts

type Writer struct {
	b []byte // packets buffer
	m int    // crc start

	pid        []uint16
	counter    []byte
	streamType []byte
	timestamp  []uint32
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) AddPES(pid uint16, streamType byte) {
	w.pid = append(w.pid, pid)
	w.streamType = append(w.streamType, streamType)
	w.counter = append(w.counter, 0)
	w.timestamp = append(w.timestamp, 0)
}

func (w *Writer) WriteByte(b byte) {
	w.b = append(w.b, b)
}

func (w *Writer) WriteUint16(i uint16) {
	w.b = append(w.b, byte(i>>8), byte(i))
}

func (w *Writer) WriteTime(t uint32) {
	const onlyPTS = 0x20
	// [>>32 <<3] [>>24 <<2] [>>16 <<2] [>>8 <<1] [<<1]
	w.b = append(w.b, onlyPTS|byte(t>>29)|1, byte(t>>22), byte(t>>14)|1, byte(t>>7), byte(t<<1)|1)
}

func (w *Writer) WriteBytes(b []byte) {
	w.b = append(w.b, b...)
}

func (w *Writer) MarkChecksum() {
	w.m = len(w.b)
}

func (w *Writer) WriteChecksum() {
	crc := calcCRC32(0xFFFFFFFF, w.b[w.m:])
	w.b = append(w.b, byte(crc), byte(crc>>8), byte(crc>>16), byte(crc>>24))
}

func (w *Writer) FinishPacket() {
	if n := len(w.b) % PacketSize; n != 0 {
		w.b = append(w.b, make([]byte, PacketSize-n)...)
	}
}

func (w *Writer) Bytes() []byte {
	if len(w.b)%PacketSize != 0 {
		panic("wrong packet size")
	}
	return w.b
}

func (w *Writer) Reset() {
	w.b = nil
}

const isUnitStart = 0x4000
const flagHasAdaptation = 0x20
const flagHasPayload = 0x10
const lenIsProgramTable = 0xB000
const tableFlags = 0xC1
const tableHeader = 0xE000
const tableLength = 0xF000

const patPID = 0
const patTableID = 0
const patTableExtID = 1

func (w *Writer) WritePAT() {
	w.WriteByte(SyncByte)
	w.WriteUint16(isUnitStart | patPID) // PAT PID
	w.WriteByte(flagHasPayload)         // flags...

	w.WriteByte(0) // Pointer field

	w.MarkChecksum()
	w.WriteByte(patTableID)               // Table ID
	w.WriteUint16(lenIsProgramTable | 13) // Section length
	w.WriteUint16(patTableExtID)          // Table ID extension
	w.WriteByte(tableFlags)               // flags...
	w.WriteByte(0)                        // Section number
	w.WriteByte(0)                        // Last section number

	w.WriteUint16(1) // Program num (usual 1)
	w.WriteUint16(tableHeader + pmtPID)

	w.WriteChecksum()

	w.FinishPacket()
}

const pmtPID = 18
const pmtTableID = 2
const pmtTableExtID = 1

func (w *Writer) WritePMT() {
	w.WriteByte(SyncByte)
	w.WriteUint16(isUnitStart | pmtPID) // PMT PID
	w.WriteByte(flagHasPayload)         // flags...

	w.WriteByte(0) // Pointer field

	tableLen := 13 + uint16(len(w.pid))*5

	w.MarkChecksum()
	w.WriteByte(pmtTableID)                     // Table ID
	w.WriteUint16(lenIsProgramTable | tableLen) // Section length
	w.WriteUint16(pmtTableExtID)                // Table ID extension
	w.WriteByte(tableFlags)                     // flags...
	w.WriteByte(0)                              // Section number
	w.WriteByte(0)                              // Last section number

	w.WriteUint16(tableHeader | w.pid[0]) // PID
	w.WriteUint16(tableLength | 0)        // Info length

	for i, pid := range w.pid {
		w.WriteByte(w.streamType[i])
		w.WriteUint16(tableHeader | pid) // PID
		w.WriteUint16(tableLength | 0)   // Info len
	}

	w.WriteChecksum()

	w.FinishPacket()
}

const pesHeaderSize = PacketSize - 18

func (w *Writer) WritePES(pid uint16, streamID byte, payload []byte) {
	w.WriteByte(SyncByte)
	w.WriteUint16(isUnitStart | pid)

	// check if payload lower then max first packet size
	if len(payload) < PacketSize-18 {
		w.WriteByte(flagHasAdaptation | flagHasPayload)

		// for 183 payload will be zero
		adSize := PacketSize - 18 - 1 - byte(len(payload))
		w.WriteByte(adSize)
		w.WriteBytes(make([]byte, adSize))
	} else {
		w.WriteByte(flagHasPayload)
	}

	w.WriteByte(0)
	w.WriteByte(0)
	w.WriteByte(1)

	w.WriteByte(streamID)
	w.WriteUint16(uint16(8 + len(payload)))

	w.WriteByte(0x80)
	w.WriteByte(0x80) // only PTS
	w.WriteByte(5)    // optional size

	switch w.streamType[0] {
	case StreamTypePCMATapo:
		w.timestamp[0] += uint32(len(payload) * 45 / 8)
	}

	w.WriteTime(w.timestamp[0])

	if len(payload) < PacketSize-18 {
		w.WriteBytes(payload)
		return
	}

	w.WriteBytes(payload[:pesHeaderSize])

	payload = payload[pesHeaderSize:]
	var counter byte

	for {
		counter++

		if len(payload) > PacketSize-4 {
			// payload more then maximum size
			w.WriteByte(SyncByte)
			w.WriteUint16(pid)
			w.WriteByte(flagHasPayload | counter&0xF)
			w.WriteBytes(payload[:PacketSize-4])

			payload = payload[PacketSize-4:]
		} else if len(payload) == PacketSize-4 {
			// payload equal maximum size (last packet)
			w.WriteByte(SyncByte)
			w.WriteUint16(pid)
			w.WriteByte(flagHasPayload | counter&0xF)
			w.WriteBytes(payload)

			break
		} else {
			// payload lower than maximum size (last packet)
			w.WriteByte(SyncByte)
			w.WriteUint16(pid)
			w.WriteByte(flagHasAdaptation | flagHasPayload | counter&0xF)

			// for 183 payload will be zero
			adSize := PacketSize - 4 - 1 - byte(len(payload))
			w.WriteByte(adSize)
			w.WriteBytes(make([]byte, adSize))

			w.WriteBytes(payload)

			break
		}
	}
}
