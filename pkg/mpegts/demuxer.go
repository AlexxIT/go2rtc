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

	pmtID uint16 // Program Map Table (PMT) PID
	pes   map[uint16]*PES
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

		if d.pes == nil {
			switch pid {
			case 0: // PAT ID
				d.readPAT() // PAT: Program Association Table
			case d.pmtID:
				d.readPMT() // PMT : Program Map Table

				pkt := &rtp.Packet{
					Payload: make([]byte, 0, len(d.pes)),
				}
				for _, pes := range d.pes {
					pkt.Payload = append(pkt.Payload, pes.StreamType)
				}
				return pkt, nil
			}
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

func (d *Demuxer) readPSIHeader() {
	// https://en.wikipedia.org/wiki/Program-specific_information#Table_Sections
	pointer := d.readByte() // Pointer field
	d.skip(pointer)         // Pointer filler bytes

	_ = d.readByte()       // Table ID
	_ = d.readBit()        // Section syntax indicator
	_ = d.readBit()        // Private bit
	_ = d.readBits(2)      // Reserved bits
	_ = d.readBits(2)      // Section length unused bits
	size := d.readBits(10) // Section length
	d.setSize(byte(size))

	_ = d.readBits(16) // Table ID extension
	_ = d.readBits(2)  // Reserved bits
	_ = d.readBits(5)  // Version number
	_ = d.readBit()    // Current/next indicator
	_ = d.readByte()   // Section number
	_ = d.readByte()   // Last section number
}

// ReadPAT (Program Association Table)
func (d *Demuxer) readPAT() {
	// https://en.wikipedia.org/wiki/Program-specific_information#PAT_(Program_Association_Table)
	d.readPSIHeader()

	const CRCSize = 4
	for d.left() > CRCSize {
		num := d.readBits(16)   // Program num
		_ = d.readBits(3)       // Reserved bits
		pid := d.readBits16(13) // Program map PID
		if num != 0 {
			d.pmtID = pid
		}
	}

	d.skip(4) // CRC32
}

// ReadPMT (Program map specific data)
func (d *Demuxer) readPMT() {
	// https://en.wikipedia.org/wiki/Program-specific_information#PMT_(Program_map_specific_data)
	d.readPSIHeader()

	_ = d.readBits(3)      // Reserved bits
	_ = d.readBits(13)     // PCR PID
	_ = d.readBits(4)      // Reserved bits
	_ = d.readBits(2)      // Program info length unused bits
	size := d.readBits(10) // Program info length
	d.skip(byte(size))

	d.pes = map[uint16]*PES{}

	const CRCSize = 4
	for d.left() > CRCSize {
		streamType := d.readByte() // Stream type
		_ = d.readBits(3)          // Reserved bits
		pid := d.readBits16(13)    // Elementary PID
		_ = d.readBits(4)          // Reserved bits
		_ = d.readBits(2)          // ES Info length unused bits
		size = d.readBits(10)      // ES Info length
		info := d.readBytes(byte(size))

		if streamType == StreamTypePrivate && bytes.HasPrefix(info, opusInfo) {
			streamType = StreamTypePrivateOPUS
		}

		d.pes[pid] = &PES{StreamType: streamType}
	}

	d.skip(4) // CRC32
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
			Payload: annexb.EncodeToAVCC(p.Payload, false),
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

	case StreamTypePCMATapo:
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
