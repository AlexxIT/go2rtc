package mov

const (
	MoovTrakMdiaMinfStblStsdAvc1     = "avc1"
	MoovTrakMdiaMinfStblStsdAvc1AvcC = "avcC"
	MoovTrakMdiaMinfStblStsdHev1     = "hev1"
	MoovTrakMdiaMinfStblStsdHev1HvcC = "hvcC"
	MoovTrakMdiaMinfStblStsdMp4a     = "mp4a"
)

func (m *Movie) WriteH26X(width, height uint16, conf []byte, h264 bool) {
	// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/QTFFChap3/qtff3.html
	if h264 {
		m.StartAtom(MoovTrakMdiaMinfStblStsdAvc1)
	} else {
		m.StartAtom(MoovTrakMdiaMinfStblStsdHev1)
	}
	m.Skip(6)
	m.WriteUint16(1)      // data_reference_index
	m.Skip(2)             // version
	m.Skip(2)             // revision
	m.Skip(4)             // vendor
	m.Skip(4)             // temporal quality
	m.Skip(4)             // spatial quality
	m.WriteUint16(width)  // width
	m.WriteUint16(height) // height
	m.WriteFloat32(72)    // horizontal resolution
	m.WriteFloat32(72)    // vertical resolution
	m.Skip(4)             // reserved
	m.WriteUint16(1)      // frame count
	m.Skip(32)            // compressor name
	m.WriteUint16(24)     // depth
	m.WriteUint16(0xFFFF) // color table id (-1)

	if h264 {
		m.StartAtom(MoovTrakMdiaMinfStblStsdAvc1AvcC)
	} else {
		m.StartAtom(MoovTrakMdiaMinfStblStsdHev1HvcC)
	}
	m.Write(conf)
	m.EndAtom() // AVCC

	m.EndAtom() // AVC1
}

func (m *Movie) WriteMP4A(channels, sampleSize uint16, sampleRate uint32, conf []byte) {
	m.StartAtom(MoovTrakMdiaMinfStblStsdMp4a)
	m.Skip(6)
	m.WriteUint16(1)                    // data_reference_index
	m.Skip(2)                           // version
	m.Skip(2)                           // revision
	m.Skip(4)                           // vendor
	m.WriteUint16(channels)             // channel_count
	m.WriteUint16(sampleSize)           // sample_size
	m.Skip(2)                           // compression id
	m.Skip(2)                           // reserved
	m.WriteFloat32(float64(sampleRate)) // sample_rate

	m.WriteESDS(conf)

	m.EndAtom() // MP4A
}

func (m *Movie) WriteESDS(conf []byte) {
	m.StartAtom("esds")
	m.Skip(1) // version
	m.Skip(3) // flags

	// MP4ESDescrTag[3]:
	// - MP4DecConfigDescrTag[4]:
	//   - MP4DecSpecificDescrTag[5]: conf
	// - Other[6]
	const header = 5
	const size3 = 3
	const size4 = 13
	size5 := byte(len(conf))
	const size6 = 1

	m.WriteBytes(3, 0x80, 0x80, 0x80, size3+header+size4+header+size5+header+size6)
	m.Skip(2) // es id
	m.Skip(1) // es flags

	m.WriteBytes(4, 0x80, 0x80, 0x80, size4+header+size5)
	m.WriteBytes(0x40) // object id
	m.WriteBytes(0x15) // stream type
	m.Skip(3)          // buffer size db
	m.Skip(4)          // max bitraga
	m.Skip(4)          // avg bitraga

	m.WriteBytes(5, 0x80, 0x80, 0x80, size5)
	m.Write(conf)

	m.WriteBytes(6, 0x80, 0x80, 0x80, 1)
	m.WriteBytes(2) // ?

	m.EndAtom() // ESDS
}
