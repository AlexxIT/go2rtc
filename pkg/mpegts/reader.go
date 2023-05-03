package mpegts

import "github.com/pion/rtp"

type Reader struct {
	b []byte // packets buffer
	i byte   // read position
	s byte   // end position

	pmt uint16 // Program Map Table (PMT) PID
	pes map[uint16]*PES
}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) SetBuffer(b []byte) {
	r.b = b
	r.i = 0
	r.s = PacketSize
}

func (r *Reader) AppendBuffer(b []byte) {
	r.b = append(r.b, b...)
}

func (r *Reader) GetPacket() *rtp.Packet {
	for r.Sync() {
		r.Skip(1) // Sync byte

		pid := r.ReadUint16() & 0x1FFF // PID
		flag := r.ReadByte()           // flags...

		const pidNullPacket = 0x1FFF
		if pid == pidNullPacket {
			continue
		}

		const hasAdaptionField = 0b0010_0000
		if flag&hasAdaptionField != 0 {
			adSize := r.ReadByte() // Adaptation field length
			if adSize > PacketSize-6 {
				println("WARNING: mpegts: wrong adaptation size")
				continue
			}
			r.Skip(adSize)
		}

		// PAT: Program Association Table
		const pidPAT = 0
		if pid == pidPAT {
			// already processed
			if r.pmt != 0 {
				continue
			}

			r.ReadPSIHeader()

			const CRCSize = 4
			for r.Left() > CRCSize {
				pNum := r.ReadUint16()
				pPID := r.ReadUint16() & 0x1FFF
				if pNum != 0 {
					r.pmt = pPID
				}
			}

			r.Skip(4) // CRC32
			continue
		}

		// PMT : Program Map Table
		if pid == r.pmt {
			// already processed
			if r.pes != nil {
				continue
			}

			r.ReadPSIHeader()

			pesPID := r.ReadUint16() & 0x1FFF // ? PCR PID
			pSize := r.ReadUint16() & 0x03FF  // ? 0x0FFF
			r.Skip(byte(pSize))

			r.pes = map[uint16]*PES{}

			const CRCSize = 4
			for r.Left() > CRCSize {
				streamType := r.ReadByte()
				pesPID = r.ReadUint16() & 0x1FFF // Elementary PID
				iSize := r.ReadUint16() & 0x03FF // ? 0x0FFF
				r.Skip(byte(iSize))

				r.pes[pesPID] = &PES{StreamType: streamType}
			}

			r.Skip(4) // ? CRC32
			continue
		}

		if r.pes == nil {
			continue
		}

		pes := r.pes[pid]
		if pes == nil {
			continue // unknown PID
		}

		if pes.Payload == nil {
			// PES Packet start code prefix
			if r.ReadByte() != 0 || r.ReadByte() != 0 || r.ReadByte() != 1 {
				continue
			}

			// read stream ID and total payload size
			pes.StreamID = r.ReadByte()
			pes.SetBuffer(r.ReadUint16(), r.Bytes())
		} else {
			pes.AppendBuffer(r.Bytes())
		}

		if pkt := pes.GetPacket(); pkt != nil {
			return pkt
		}
	}

	return nil
}

func (r *Reader) GetStreamTypes() []byte {
	types := make([]byte, 0, len(r.pes))
	for _, pes := range r.pes {
		types = append(types, pes.StreamType)
	}
	return types
}

// Sync - search sync byte
func (r *Reader) Sync() bool {
	// drop previous readed packet
	if r.i != 0 {
		r.b = r.b[PacketSize:]
		r.i = 0
		r.s = PacketSize
	}

	// if packet available
	if len(r.b) < PacketSize {
		return false
	}

	// if data starts from sync byte
	if r.b[0] == SyncByte {
		return true
	}

	for len(r.b) >= PacketSize {
		if r.b[0] == SyncByte {
			return true
		}
		r.b = r.b[1:]
	}

	return false
}

func (r *Reader) ReadPSIHeader() {
	pointer := r.ReadByte() // Pointer field
	r.Skip(pointer)         // Pointer filler bytes

	r.Skip(1)                       // Table ID
	size := r.ReadUint16() & 0x03FF // Section length
	r.SetSize(byte(size))

	r.Skip(2) // Table ID extension
	r.Skip(1) // flags...
	r.Skip(1) // Section number
	r.Skip(1) // Last section number
}

func (r *Reader) Skip(i byte) {
	r.i += i
}

func (r *Reader) ReadByte() byte {
	b := r.b[r.i]
	r.i++
	return b
}

func (r *Reader) ReadUint16() uint16 {
	i := (uint16(r.b[r.i]) << 8) | uint16(r.b[r.i+1])
	r.i += 2
	return i
}

func (r *Reader) Bytes() []byte {
	return r.b[r.i:PacketSize]
}

func (r *Reader) Left() byte {
	return r.s - r.i
}

func (r *Reader) SetSize(size byte) {
	r.s = r.i + size
}
