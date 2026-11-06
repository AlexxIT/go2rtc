package mpegts

import (
	"bytes"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

type Demuxer struct {
	buf [PacketSize]byte // total buf

	byte byte // current byte
	bits byte // bits left in byte
	pos  byte // current pos in buf
	end  byte // end position

	pmtIDs map[uint16]bool // Program Map Table (PMT) PIDs, one per program in the PAT
	pes    map[uint16]*PES
}

func NewDemuxer() *Demuxer {
	return &Demuxer{}
}

const skipRead = 0xFF

func (d *Demuxer) ReadPacket(rd io.Reader) (*rtp.Packet, error) {
	for {
		if d.pos != skipRead {
			if _, err := io.ReadFull(rd, d.buf[:]); err != nil {
				return nil, err
			}
		}

		pid, start, err := d.readPacketHeader()
		if err != nil {
			return nil, err
		}

		switch {
		case pid == 0: // PAT ID
			if start && d.readPSIHeader(tableIDPAT) {
				d.readPAT() // PAT: Program Association Table
			}
			continue
		case d.pmtIDs[pid]:
			// PMT : Program Map Table. Keep reading it after the first one: some sources (Tapo) announce a stream
			// in a later PMT, and its PID must not be silently dropped. Only section starts are parsed: a PSI
			// section spanning several packets is not supported (and was not before either).
			if !start || !d.readPSIHeader(tableIDPMT) {
				continue
			}

			first := d.pes == nil
			d.readPMT(!first)
			if !first {
				continue
			}

			pkt := &rtp.Packet{
				Payload: make([]byte, 0, len(d.pes)),
			}
			for _, pes := range d.pes {
				pkt.Payload = append(pkt.Payload, pes.StreamType)
			}
			return pkt, nil
		case d.pes == nil:
			continue
		}

		if pkt := d.readPES(pid, start); pkt != nil {
			return pkt, nil
		}
	}
}

func (d *Demuxer) readPacketHeader() (pid uint16, start bool, err error) {
	d.reset()

	sb := d.readByte() // Sync byte
	if sb != SyncByte {
		return 0, false, errors.New("mpegts: wrong sync byte")
	}

	_ = d.readBit()        // Transport error indicator (TEI)
	pusi := d.readBit()    // Payload unit start indicator (PUSI)
	_ = d.readBit()        // Transport priority
	pid = d.readBits16(13) // PID

	_ = d.readBits(2) // Transport scrambling control (TSC)
	af := d.readBit() // Adaptation field
	_ = d.readBit()   // Payload
	_ = d.readBits(4) // Continuity counter

	if af != 0 {
		adSize := d.readByte() // Adaptation field length
		if adSize > PacketSize-6 {
			return 0, false, errors.New("mpegts: wrong adaptation size")
		}
		d.skip(adSize)
	}

	return pid, pusi != 0, nil
}

func (d *Demuxer) skip(i byte) {
	d.pos += i
}

func (d *Demuxer) readBytes(i byte) []byte {
	d.pos += i
	return d.buf[d.pos-i : d.pos]
}

const (
	tableIDPAT = 0x00
	tableIDPMT = 0x02
)

// readPSIHeader reads a table section header and reports whether a complete section of the expected table follows
// in this packet. It never reads past the packet: a bad pointer or length drops the section instead of panicking.
func (d *Demuxer) readPSIHeader(tableID byte) bool {
	// https://en.wikipedia.org/wiki/Program-specific_information#Table_Sections
	pointer := d.readByte() // Pointer field
	if int(d.pos)+int(pointer)+3 > PacketSize {
		return false
	}
	d.skip(pointer) // Pointer filler bytes

	if d.readByte() != tableID { // Table ID
		return false
	}
	_ = d.readBit()        // Section syntax indicator
	_ = d.readBit()        // Private bit
	_ = d.readBits(2)      // Reserved bits
	_ = d.readBits(2)      // Section length unused bits
	size := d.readBits(10) // Section length
	const headerSize, crcSize = 5, 4
	if size < headerSize+crcSize || int(d.pos)+int(size) > PacketSize {
		return false
	}
	d.setSize(byte(size))

	_ = d.readBits(16) // Table ID extension
	_ = d.readBits(2)  // Reserved bits
	_ = d.readBits(5)  // Version number
	_ = d.readBit()    // Current/next indicator
	_ = d.readByte()   // Section number
	_ = d.readByte()   // Last section number
	return true
}

// ReadPAT (Program Association Table)
func (d *Demuxer) readPAT() {
	// https://en.wikipedia.org/wiki/Program-specific_information#PAT_(Program_Association_Table)

	const CRCSize = 4
	for d.left() > CRCSize {
		num := d.readBits(16)   // Program num
		_ = d.readBits(3)       // Reserved bits
		pid := d.readBits16(13) // Program map PID
		if num != 0 {
			if d.pmtIDs == nil {
				d.pmtIDs = map[uint16]bool{}
			}
			d.pmtIDs[pid] = true
		}
	}

	d.skip(4) // CRC32
}

// ReadPMT (Program map specific data). With merge, a stream announced by a later PMT is added only if no known
// stream already has its type (consumers route packets by type, so two PIDs of one type would interleave), and a
// PID whose type changed is reset.
func (d *Demuxer) readPMT(merge bool) {
	// https://en.wikipedia.org/wiki/Program-specific_information#PMT_(Program_map_specific_data)
	_ = d.readBits(3)      // Reserved bits
	_ = d.readBits(13)     // PCR PID
	_ = d.readBits(4)      // Reserved bits
	_ = d.readBits(2)      // Program info length unused bits
	size := d.readBits(10) // Program info length
	if int(size) > int(d.left()) {
		return
	}
	d.skip(byte(size))

	if d.pes == nil {
		d.pes = map[uint16]*PES{}
	}

	const CRCSize = 4
	for d.left() > CRCSize {
		streamType := d.readByte() // Stream type
		_ = d.readBits(3)          // Reserved bits
		pid := d.readBits16(13)    // Elementary PID
		_ = d.readBits(4)          // Reserved bits
		_ = d.readBits(2)          // ES Info length unused bits
		size = d.readBits(10)      // ES Info length
		if int(size) > int(d.left()) {
			return
		}
		info := d.readBytes(byte(size))

		if streamType == StreamTypePrivate && bytes.HasPrefix(info, opusInfo) {
			streamType = StreamTypePrivateOPUS
		}

		switch pes, ok := d.pes[pid]; {
		case !merge:
			d.pes[pid] = &PES{StreamType: streamType}
		case ok && pes.StreamType != streamType:
			d.pes[pid] = &PES{StreamType: streamType} // drop the partial payload of the old type
		case !ok && !d.hasStreamType(streamType):
			d.pes[pid] = &PES{StreamType: streamType}
		}
	}

	d.skip(4) // CRC32
}

func (d *Demuxer) hasStreamType(streamType byte) bool {
	for _, pes := range d.pes {
		if pes.StreamType == streamType {
			return true
		}
	}
	return false
}

func (d *Demuxer) readPES(pid uint16, start bool) *rtp.Packet {
	pes := d.pes[pid]
	if pes == nil {
		return nil
	}

	// if new payload beging
	if start {
		if len(pes.Payload) != 0 {
			d.pos = skipRead
			return pes.GetPacket() // finish previous packet
		}

		// https://en.wikipedia.org/wiki/Packetized_elementary_stream
		// Packet start code prefix
		if d.readByte() != 0 || d.readByte() != 0 || d.readByte() != 1 {
			return nil
		}

		pes.StreamID = d.readByte()    // Stream id
		packetSize := d.readBits16(16) // PES Packet length

		_ = d.readBits(2) // Marker bits
		_ = d.readBits(2) // Scrambling control
		_ = d.readBit()   // Priority
		_ = d.readBit()   // Data alignment indicator
		_ = d.readBit()   // Copyright
		_ = d.readBit()   // Original or Copy

		ptsi := d.readBit() // PTS indicator
		dtsi := d.readBit() // DTS indicator
		_ = d.readBit()     // ESCR flag
		_ = d.readBit()     // ES rate flag
		_ = d.readBit()     // DSM trick mode flag
		_ = d.readBit()     // Additional copy info flag
		_ = d.readBit()     // CRC flag
		_ = d.readBit()     // extension flag

		headerSize := d.readByte() // PES header length

		if packetSize != 0 {
			packetSize -= uint16(3 + headerSize)
		}

		if ptsi != 0 {
			pes.PTS = d.readTime()
			headerSize -= 5
		} else {
			pes.PTS = 0
		}

		if dtsi != 0 {
			pes.DTS = d.readTime()
			headerSize -= 5
		} else {
			pes.DTS = 0
		}

		d.skip(headerSize)

		pes.SetBuffer(packetSize, d.bytes())
	} else {
		pes.AppendBuffer(d.bytes())
	}

	if pes.Size != 0 && len(pes.Payload) >= pes.Size {
		return pes.GetPacket() // finish current packet
	}

	return nil
}

func (d *Demuxer) reset() {
	d.pos = 0
	d.end = PacketSize
	d.bits = 0
}

//goland:noinspection GoStandardMethods
func (d *Demuxer) readByte() byte {
	if d.bits != 0 {
		return byte(d.readBits(8))
	}

	b := d.buf[d.pos]
	d.pos++
	return b
}

func (d *Demuxer) readBit() byte {
	if d.bits == 0 {
		d.byte = d.readByte()
		d.bits = 7
	} else {
		d.bits--
	}

	return (d.byte >> d.bits) & 0b1
}

func (d *Demuxer) readBits(n byte) (res uint32) {
	for i := n - 1; i != 255; i-- {
		res |= uint32(d.readBit()) << i
	}
	return
}

func (d *Demuxer) readBits16(n byte) (res uint16) {
	for i := n - 1; i != 255; i-- {
		res |= uint16(d.readBit()) << i
	}
	return
}

func (d *Demuxer) readTime() uint32 {
	// https://en.wikipedia.org/wiki/Packetized_elementary_stream
	// xxxxAAAx BBBBBBBB BBBBBBBx CCCCCCCC CCCCCCCx
	_ = d.readBits(4) // 0010b or 0011b or 0001b
	ts := d.readBits(3) << 30
	_ = d.readBits(1) // 1b
	ts |= d.readBits(15) << 15
	_ = d.readBits(1) // 1b
	ts |= d.readBits(15)
	_ = d.readBits(1) // 1b
	return ts
}

func (d *Demuxer) bytes() []byte {
	return d.buf[d.pos:PacketSize]
}

func (d *Demuxer) left() byte {
	return d.end - d.pos
}

func (d *Demuxer) setSize(size byte) {
	d.end = d.pos + size
}

const (
	PacketSize = 188
	SyncByte   = 0x47  // Uppercase G
	ClockRate  = 90000 // fixed clock rate for PTS/DTS of any type
)

// https://en.wikipedia.org/wiki/Program-specific_information#Elementary_stream_types
const (
	StreamTypeMetadata    = 0    // Reserved
	StreamTypePrivate     = 0x06 // PCMU or PCMA or FLAC from FFmpeg
	StreamTypeAAC         = 0x0F
	StreamTypeH264        = 0x1B
	StreamTypeH265        = 0x24
	StreamTypePCMATapo    = 0x90
	StreamTypePCMUTapo    = 0x91
	StreamTypePrivateOPUS = 0xEB
)

// PES - Packetized Elementary Stream
type PES struct {
	StreamID   byte   // from each PES header
	StreamType byte   // from PMT table
	Sequence   uint16 // manual
	Timestamp  uint32 // manual
	PTS        uint32 // from extra header, always 90000Hz
	DTS        uint32
	Payload    []byte // from PES body
	Size       int    // from PES header, can be 0

	wr *bits.Writer
}

func (p *PES) SetBuffer(size uint16, b []byte) {
	p.Payload = make([]byte, 0, size)
	p.Payload = append(p.Payload, b...)
	p.Size = int(size)
}

func (p *PES) AppendBuffer(b []byte) {
	p.Payload = append(p.Payload, b...)
}

func (p *PES) GetPacket() (pkt *rtp.Packet) {
	switch p.StreamType {
	case StreamTypeH264, StreamTypeH265:
		pkt = &rtp.Packet{
			Header: rtp.Header{
				PayloadType: p.StreamType,
			},
			Payload: annexb.EncodeToAVCC(p.Payload),
		}

		if p.DTS != 0 {
			pkt.Timestamp = p.DTS
			// wrong place for CTS, but we don't have another one
			pkt.ExtensionProfile = uint16(p.PTS - p.DTS)
		} else {
			pkt.Timestamp = p.PTS
		}

	case StreamTypeAAC:
		p.Sequence++

		pkt = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    p.StreamType,
				SequenceNumber: p.Sequence,
				Timestamp:      p.PTS,
				//Timestamp:      p.Timestamp,
			},
			Payload: aac.ADTStoRTP(p.Payload),
		}

		//p.Timestamp += aac.RTPTimeSize(pkt.Payload) // update next timestamp!

	case StreamTypePCMATapo, StreamTypePCMUTapo:
		p.Sequence++

		pkt = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    p.StreamType,
				SequenceNumber: p.Sequence,
				Timestamp:      p.PTS,
				//Timestamp:      p.Timestamp,
			},
			Payload: p.Payload,
		}

		//p.Timestamp += uint32(len(p.Payload)) // update next timestamp!

	case StreamTypePrivateOPUS:
		p.Sequence++

		pkt = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    p.StreamType,
				SequenceNumber: p.Sequence,
				Timestamp:      p.PTS,
			},
		}

		pkt.Payload, p.Payload = CutOPUSPacket(p.Payload)
		p.PTS += opusDT
		return
	}

	p.Payload = nil

	return
}
