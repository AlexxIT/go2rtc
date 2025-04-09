package iso

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/bits"
)

type Atom struct {
	Name string
	Data []byte
}

type AtomTkhd struct {
	TrackID uint32
}

type AtomMdhd struct {
	TimeScale uint32
}

type AtomVideo struct {
	Name   string
	Config []byte
}

type AtomAudio struct {
	Name       string
	Channels   uint16
	SampleRate uint32
	Config     []byte
}

type AtomMfhd struct {
	Sequence uint32
}

type AtomMdat struct {
	Data []byte
}

type AtomTfhd struct {
	TrackID        uint32
	SampleDuration uint32
	SampleSize     uint32
	SampleFlags    uint32
}
type AtomTfdt struct {
	DecodeTime uint64
}

type AtomTrun struct {
	DataOffset       uint32
	FirstSampleFlags uint32
	SamplesDuration  []uint32
	SamplesSize      []uint32
	SamplesFlags     []uint32
	SamplesCTS       []uint32
}

func DecodeAtom(b []byte) (any, error) {
	size := binary.BigEndian.Uint32(b)
	if len(b) < int(size) {
		return nil, io.EOF
	}

	name := string(b[4:8])
	data := b[8:size]

	switch name {
	// useful containers
	case Moov, MoovTrak, MoovTrakMdia, MoovTrakMdiaMinf, MoovTrakMdiaMinfStbl, Moof, MoofTraf:
		return DecodeAtoms(data)

	case MoovTrakTkhd:
		return &AtomTkhd{TrackID: binary.BigEndian.Uint32(data[1+3+4+4:])}, nil

	case MoovTrakMdiaMdhd:
		return &AtomMdhd{TimeScale: binary.BigEndian.Uint32(data[1+3+4+4:])}, nil

	case MoovTrakMdiaMinfStblStsd:
		// support only 1 codec entry
		if n := binary.BigEndian.Uint32(data[1+3:]); n == 1 {
			return DecodeAtom(data[1+3+4:])
		}

	case "avc1", "hev1":
		b = data[6+2+2+2+4+4+4+2+2+4+4+4+2+32+2+2:]
		atom, err := DecodeAtom(b)
		if err != nil {
			return nil, err
		}
		if conf, ok := atom.(*Atom); ok {
			return &AtomVideo{Name: name, Config: conf.Data}, nil
		}

	case "mp4a":
		atom := &AtomAudio{Name: name}

		rd := bits.NewReader(data)
		rd.ReadBytes(6 + 2 + 2 + 2 + 4) // skip
		atom.Channels = rd.ReadUint16()
		rd.ReadBytes(2 + 2 + 2) // skip
		atom.SampleRate = uint32(rd.ReadFloat32())

		atom2, _ := DecodeAtom(rd.Left())
		if conf, ok := atom2.(*Atom); ok {
			_, b, _ = bytes.Cut(conf.Data, []byte{5, 0x80, 0x80, 0x80})
			if n := len(b); n > 0 && n > 1+int(b[0]) {
				atom.Config = b[1 : 1+b[0]]
			}
		}

		return atom, nil

	case MoofMfhd:
		return &AtomMfhd{Sequence: binary.BigEndian.Uint32(data[4:])}, nil

	case MoofTrafTfhd:
		rd := bits.NewReader(data)
		_ = rd.ReadByte() // version
		flags := rd.ReadUint24()

		atom := &AtomTfhd{
			TrackID: rd.ReadUint32(),
		}

		if flags&TfhdDefaultSampleDuration != 0 {
			atom.SampleDuration = rd.ReadUint32()

		}
		if flags&TfhdDefaultSampleSize != 0 {
			atom.SampleSize = rd.ReadUint32()
		}
		if flags&TfhdDefaultSampleFlags != 0 {
			atom.SampleFlags = rd.ReadUint32() // skip
		}

		return atom, nil

	case MoofTrafTfdt:
		return &AtomTfdt{DecodeTime: binary.BigEndian.Uint64(data[4:])}, nil

	case MoofTrafTrun:
		rd := bits.NewReader(data)
		_ = rd.ReadByte() // version
		flags := rd.ReadUint24()
		samples := rd.ReadUint32()

		atom := &AtomTrun{}

		if flags&TrunDataOffset != 0 {
			atom.DataOffset = rd.ReadUint32()
		}
		if flags&TrunFirstSampleFlags != 0 {
			atom.FirstSampleFlags = rd.ReadUint32()
		}

		for i := uint32(0); i < samples; i++ {
			if flags&TrunSampleDuration != 0 {
				atom.SamplesDuration = append(atom.SamplesDuration, rd.ReadUint32())
			}
			if flags&TrunSampleSize != 0 {
				atom.SamplesSize = append(atom.SamplesSize, rd.ReadUint32())
			}
			if flags&TrunSampleFlags != 0 {
				atom.SamplesFlags = append(atom.SamplesFlags, rd.ReadUint32())
			}
			if flags&TrunSampleCTS != 0 {
				atom.SamplesCTS = append(atom.SamplesCTS, rd.ReadUint32())
			}
		}

		return atom, nil

	case Mdat:
		return &AtomMdat{Data: data}, nil
	}

	return &Atom{Name: name, Data: data}, nil
}

func DecodeAtoms(b []byte) (atoms []any, err error) {
	for len(b) > 0 {
		atom, err := DecodeAtom(b)
		if err != nil {
			return nil, err
		}

		if childs, ok := atom.([]any); ok {
			atoms = append(atoms, childs...)
		} else {
			atoms = append(atoms, atom)
		}

		size := binary.BigEndian.Uint32(b)
		b = b[size:]
	}

	return atoms, nil
}
