package mpegts

type Reader struct {
	b []byte // packets buffer
	i byte   // read position

	pmt uint16 // Program Map Table (PMT) PID
	pes map[uint16]*PES
}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) SetBuffer(b []byte) {
	r.b = b
	r.i = 0
}

func (r *Reader) AppendBuffer(b []byte) {
	r.b = append(r.b, b...)
}

func (r *Reader) GetPacket() *Packet {
	for r.Sync() {
		r.Skip(1) // Sync byte

		pid := r.ReadUint16() & 0x1FFF // PID
		flag := r.ReadByte()           // flags...

		const hasAdaptionField = 0x20
		if flag&hasAdaptionField != 0 {
			adSize := r.ReadByte() // Adaptation field length
			if adSize > PacketSize-6 {
				println("WARNING: mpegts: wrong adaptation size")
				continue
			}
			r.Skip(adSize)
		}

		// PAT: Program Association Table
		const PAT = 0
		if pid == PAT {
			// already processed
			if r.pmt != 0 {
				continue
			}

			if size := r.ReadPSIHeader(); size <= 4 {
				println("WARNING: mpegts: wrong PAT")
				continue
			}

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

			if size := r.ReadPSIHeader(); size == 0 {
				println("WARNING: mpegts: wrong PMT")
				continue
			}

			pesPID := r.ReadUint16() & 0x1FFF
			pSize := r.ReadUint16() & 0x03FF
			r.Skip(byte(pSize))

			r.pes = map[uint16]*PES{}

			const minItemSize = 5
			for r.Left() > minItemSize {
				streamType := r.ReadByte()
				pesPID = r.ReadUint16() & 0x1FFF
				iSize := r.ReadUint16() & 0x03FF
				r.Skip(byte(iSize))

				r.pes[pesPID] = &PES{StreamType: streamType}
			}
			continue
		}

		if r.pes == nil {
			continue
		}

		pes := r.pes[pid]
		if pes == nil {
			continue // unknown PID
		}

		if pes.Payload != nil {
			// how many bytes left to collect
			left := cap(pes.Payload) - len(pes.Payload) - int(r.Left())

			// buffer overflow
			if left < 0 {
				println("WARNING: mpegts: buffer overflow")
				pes.Payload = nil
				continue
			}

			pes.Payload = append(pes.Payload, r.Bytes()...)

			if left == 0 {
				pkt := pes.Packet()
				pes.Payload = nil
				return pkt
			}

			continue
		}

		// PES Packet start code prefix
		if r.ReadByte() != 0 || r.ReadByte() != 0 || r.ReadByte() != 1 {
			continue
		}

		// read stream ID and total payload size
		pes.StreamID = r.ReadByte()
		pes.Payload = make([]byte, 0, r.ReadUint16())
		pes.Payload = append(pes.Payload, r.Bytes()...)
	}

	return nil
}

// Sync - search sync byte
func (r *Reader) Sync() bool {
	// drop previous readed packet
	if r.i != 0 {
		r.b = r.b[PacketSize:]
		r.i = 0
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

func (r *Reader) ReadPSIHeader() uint16 {
	pointer := r.ReadByte() // Pointer field
	r.Skip(pointer)         // Pointer filler bytes

	r.Skip(1)                       // Table ID
	size := r.ReadUint16() & 0x03FF // Section length

	if uint16(r.i)+size != uint16(PacketSize) {
		return 0
	}

	r.Skip(2) // Table ID extension
	r.Skip(1) // flags...
	r.Skip(1) // Section number
	r.Skip(1) // Last section number

	return size - 5
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
	return PacketSize - r.i
}
