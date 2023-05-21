package iso

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
)

func (m *Movie) WriteVideo(codec string, width, height uint16, conf []byte) {
	// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/QTFFChap3/qtff3.html
	switch codec {
	case core.CodecH264:
		m.StartAtom("avc1")
	case core.CodecH265:
		m.StartAtom("hev1")
	default:
		panic("unsupported iso video: " + codec)
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

	switch codec {
	case core.CodecH264:
		m.StartAtom("avcC")
	case core.CodecH265:
		m.StartAtom("hvcC")
	}
	m.Write(conf)
	m.EndAtom() // AVCC

	m.EndAtom() // AVC1
}

func (m *Movie) WriteAudio(codec string, channels uint16, sampleRate uint32, conf []byte) {
	switch codec {
	case core.CodecAAC, core.CodecMP3:
		m.StartAtom("mp4a") // supported in all players and browsers
	case core.CodecFLAC:
		m.StartAtom("fLaC") // supported in all players and browsers
	case core.CodecOpus:
		m.StartAtom("Opus") // supported in Chrome and Firefox
	case core.CodecPCMU:
		m.StartAtom("ulaw")
	case core.CodecPCMA:
		m.StartAtom("alaw")
	default:
		panic("unsupported iso audio: " + codec)
	}

	if channels == 0 {
		channels = 1
	}

	m.Skip(6)
	m.WriteUint16(1)                    // data_reference_index
	m.Skip(2)                           // version
	m.Skip(2)                           // revision
	m.Skip(4)                           // vendor
	m.WriteUint16(channels)             // channel_count
	m.WriteUint16(16)                   // sample_size
	m.Skip(2)                           // compression id
	m.Skip(2)                           // reserved
	m.WriteFloat32(float64(sampleRate)) // sample_rate

	switch codec {
	case core.CodecAAC:
		m.WriteEsdsAAC(conf)
	case core.CodecMP3:
		m.WriteEsdsMP3()
	case core.CodecFLAC:
		m.StartAtom("dfLa")
		m.Write(pcm.FLACHeader(false, sampleRate))
		m.EndAtom()
	case core.CodecOpus:
		// don't know what means this magic
		m.StartAtom("dOps")
		m.WriteBytes(0, 0x02, 0x01, 0x38, 0, 0, 0xBB, 0x80, 0, 0, 0)
		m.EndAtom()
	case core.CodecPCMU, core.CodecPCMA:
		// don't know what means this magic
		m.StartAtom("chan")
		m.WriteBytes(0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0)
		m.EndAtom()
	}

	m.EndAtom() // MP4A/OPUS
}

func (m *Movie) WriteEsdsAAC(conf []byte) {
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

	// https://learn.microsoft.com/en-us/windows/win32/medfound/mpeg-4-file-sink#aac-audio
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

func (m *Movie) WriteEsdsMP3() {
	m.StartAtom("esds")
	m.Skip(1) // version
	m.Skip(3) // flags

	// MP4ESDescrTag[3]:
	// - MP4DecConfigDescrTag[4]:
	// - Other[6]
	const header = 5
	const size3 = 3
	const size4 = 13
	const size6 = 1

	m.WriteBytes(3, 0x80, 0x80, 0x80, size3+header+size4+header+size6)
	m.Skip(2) // es id
	m.Skip(1) // es flags

	// https://learn.microsoft.com/en-us/windows/win32/medfound/mpeg-4-file-sink#mp3-audio
	m.WriteBytes(4, 0x80, 0x80, 0x80, size4)
	m.WriteBytes(0x6B) // object id
	m.WriteBytes(0x15) // stream type
	m.Skip(3)          // buffer size db
	m.Skip(4)          // max bitraga
	m.Skip(4)          // avg bitraga

	m.WriteBytes(6, 0x80, 0x80, 0x80, 1)
	m.WriteBytes(2) // ?

	m.EndAtom() // ESDS
}
