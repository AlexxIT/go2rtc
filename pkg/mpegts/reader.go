package mpegts

import (
	"errors"
	"io"

	"github.com/pion/rtp"
)

type Reader struct {
	buf [PacketSize]byte // total buf

	byte byte // current byte
	bits byte // bits left in byte
	pos  byte // current pos in buf
	end  byte // end position

	pmtID uint16 // Program Map Table (PMT) PID
	pes   map[uint16]*PES
}

func NewReader() *Reader {
	return &Reader{}
}

const skipRead = 0xFF

func (r *Reader) ReadPacket(rd io.Reader) (*rtp.Packet, error) {
	for {
		if r.pos != skipRead {
			if _, err := io.ReadFull(rd, r.buf[:]); err != nil {
				return nil, err
			}
		}

		pid, start, err := r.readPacketHeader()
		if err != nil {
			return nil, err
		}

		if r.pes == nil {
			switch pid {
			case 0: // PAT ID
				r.readPAT() // PAT: Program Association Table
			case r.pmtID:
				r.readPMT() // PMT : Program Map Table

				pkt := &rtp.Packet{
					Payload: make([]byte, 0, len(r.pes)),
				}
				for _, pes := range r.pes {
					pkt.Payload = append(pkt.Payload, pes.StreamType)
				}
				return pkt, nil
			}
			continue
		}

		if pkt := r.readPES(pid, start); pkt != nil {
			return pkt, nil
		}
	}
}

func (r *Reader) readPacketHeader() (pid uint16, start bool, err error) {
	r.reset()

	sb := r.readByte() // Sync byte
	if sb != SyncByte {
		return 0, false, errors.New("mpegts: wrong sync byte")
	}

	_ = r.readBit()        // Transport error indicator (TEI)
	pusi := r.readBit()    // Payload unit start indicator (PUSI)
	_ = r.readBit()        // Transport priority
	pid = r.readBits16(13) // PID

	_ = r.readBits(2) // Transport scrambling control (TSC)
	af := r.readBit() // Adaptation field
	_ = r.readBit()   // Payload
	_ = r.readBits(4) // Continuity counter

	if af != 0 {
		adSize := r.readByte() // Adaptation field length
		if adSize > PacketSize-6 {
			return 0, false, errors.New("mpegts: wrong adaptation size")
		}
		r.skip(adSize)
	}

	return pid, pusi != 0, nil
}

func (r *Reader) skip(i byte) {
	r.pos += i
}

func (r *Reader) readPSIHeader() {
	// https://en.wikipedia.org/wiki/Program-specific_information#Table_Sections
	pointer := r.readByte() // Pointer field
	r.skip(pointer)         // Pointer filler bytes

	_ = r.readByte()       // Table ID
	_ = r.readBit()        // Section syntax indicator
	_ = r.readBit()        // Private bit
	_ = r.readBits(2)      // Reserved bits
	_ = r.readBits(2)      // Section length unused bits
	size := r.readBits(10) // Section length
	r.setSize(byte(size))

	_ = r.readBits(16) // Table ID extension
	_ = r.readBits(2)  // Reserved bits
	_ = r.readBits(5)  // Version number
	_ = r.readBit()    // Current/next indicator
	_ = r.readByte()   // Section number
	_ = r.readByte()   // Last section number
}

// ReadPAT (Program Association Table)
func (r *Reader) readPAT() {
	// https://en.wikipedia.org/wiki/Program-specific_information#PAT_(Program_Association_Table)
	r.readPSIHeader()

	const CRCSize = 4
	for r.left() > CRCSize {
		num := r.readBits(16)   // Program num
		_ = r.readBits(3)       // Reserved bits
		pid := r.readBits16(13) // Program map PID
		if num != 0 {
			r.pmtID = pid
		}
	}

	r.skip(4) // CRC32
}

// ReadPMT (Program map specific data)
func (r *Reader) readPMT() {
	// https://en.wikipedia.org/wiki/Program-specific_information#PMT_(Program_map_specific_data)
	r.readPSIHeader()

	_ = r.readBits(3)      // Reserved bits
	_ = r.readBits(13)     // PCR PID
	_ = r.readBits(4)      // Reserved bits
	_ = r.readBits(2)      // Program info length unused bits
	size := r.readBits(10) // Program info length
	r.skip(byte(size))

	r.pes = map[uint16]*PES{}

	const CRCSize = 4
	for r.left() > CRCSize {
		streamType := r.readByte() // Stream type
		_ = r.readBits(3)          // Reserved bits
		pid := r.readBits16(13)    // Elementary PID
		_ = r.readBits(4)          // Reserved bits
		_ = r.readBits(2)          // ES Info length unused bits
		size = r.readBits(10)      // ES Info length
		r.skip(byte(size))

		r.pes[pid] = &PES{StreamType: streamType}
	}

	r.skip(4) // CRC32
}

func (r *Reader) readPES(pid uint16, start bool) *rtp.Packet {
	pes := r.pes[pid]
	if pes == nil {
		return nil
	}

	// if new payload beging
	if start {
		if pes.Payload != nil {
			r.pos = skipRead
			return pes.GetPacket() // finish previous packet
		}

		// https://en.wikipedia.org/wiki/Packetized_elementary_stream
		// Packet start code prefix
		if r.readByte() != 0 || r.readByte() != 0 || r.readByte() != 1 {
			return nil
		}

		pes.StreamID = r.readByte()    // Stream id
		packetSize := r.readBits16(16) // PES Packet length

		_ = r.readBits(2) // Marker bits
		_ = r.readBits(2) // Scrambling control
		_ = r.readBit()   // Priority
		_ = r.readBit()   // Data alignment indicator
		_ = r.readBit()   // Copyright
		_ = r.readBit()   // Original or Copy

		pts := r.readBit() // PTS indicator
		_ = r.readBit()    // DTS indicator
		_ = r.readBit()    // ESCR flag
		_ = r.readBit()    // ES rate flag
		_ = r.readBit()    // DSM trick mode flag
		_ = r.readBit()    // Additional copy info flag
		_ = r.readBit()    // CRC flag
		_ = r.readBit()    // extension flag

		headerSize := r.readByte() // PES header length

		//log.Printf("[mpegts] pes=%d size=%d header=%d", pes.StreamID, packetSize, headerSize)

		if packetSize != 0 {
			packetSize -= uint16(3 + headerSize)
		}

		if pts != 0 {
			pes.PTS = r.readTime()
			headerSize -= 5
		}

		r.skip(headerSize)

		pes.SetBuffer(packetSize, r.bytes())
	} else {
		pes.AppendBuffer(r.bytes())
	}

	if pes.Size != 0 && len(pes.Payload) >= pes.Size {
		return pes.GetPacket() // finish current packet
	}

	return nil
}

func (r *Reader) reset() {
	r.pos = 0
	r.end = PacketSize
	r.bits = 0
}

//goland:noinspection GoStandardMethods
func (r *Reader) readByte() byte {
	if r.bits != 0 {
		return byte(r.readBits(8))
	}

	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *Reader) readBit() byte {
	if r.bits == 0 {
		r.byte = r.readByte()
		r.bits = 7
	} else {
		r.bits--
	}

	return (r.byte >> r.bits) & 0b1
}

func (r *Reader) readBits(n byte) (res uint32) {
	for i := n - 1; i != 255; i-- {
		res |= uint32(r.readBit()) << i
	}
	return
}

func (r *Reader) readBits16(n byte) (res uint16) {
	for i := n - 1; i != 255; i-- {
		res |= uint16(r.readBit()) << i
	}
	return
}

func (r *Reader) readTime() uint32 {
	// https://en.wikipedia.org/wiki/Packetized_elementary_stream
	// xxxxAAAx BBBBBBBB BBBBBBBx CCCCCCCC CCCCCCCx
	_ = r.readBits(4) // 0010b or 0011b or 0001b
	ts := r.readBits(3) << 30
	_ = r.readBits(1) // 1b
	ts |= r.readBits(15) << 15
	_ = r.readBits(1) // 1b
	ts |= r.readBits(15)
	_ = r.readBits(1) // 1b
	return ts
}

func (r *Reader) bytes() []byte {
	return r.buf[r.pos:PacketSize]
}

func (r *Reader) left() byte {
	return r.end - r.pos
}

func (r *Reader) setSize(size byte) {
	r.end = r.pos + size
}

// Deprecated:
func (r *Reader) SetBuffer(b []byte) {

}

// Deprecated:
func (r *Reader) GetPacket() *rtp.Packet {
	panic("")
}

// Deprecated:
func (r *Reader) AppendBuffer(sniff []byte) {

}
