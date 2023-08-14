package iso

import (
	"encoding/binary"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/bits"
)

type Atom struct {
	Name string
	Data []byte

	DecodeTime uint64

	SamplesDuration []uint32
	SamplesSize     []uint32
}

func DecodeAtoms(b []byte) ([]*Atom, error) {
	var atoms []*Atom
	for len(b) > 8 {
		size := binary.BigEndian.Uint32(b)
		if uint32(len(b)) < size {
			return nil, io.EOF
		}

		name := string(b[4:8])
		data := b[8:size]

		b = b[size:]

		switch name {
		case Moof, MoofTraf:
			childs, err := DecodeAtoms(data)
			if err != nil {
				return nil, err
			}

			atoms = append(atoms, childs...)

		case MoofMfhd, MoofTrafTfhd:
			continue

		case MoofTrafTfdt:
			if len(data) < 8 {
				return nil, io.EOF
			}

			dt := binary.BigEndian.Uint64(data[4:])
			atoms = append(atoms, &Atom{Name: name, DecodeTime: dt})

		case MoofTrafTrun:
			rd := bits.NewReader(data)

			_ = rd.ReadByte() // version
			flags := rd.ReadUint24()
			samples := rd.ReadUint32()

			if flags&TrunDataOffset != 0 {
				_ = rd.ReadUint32() // skip
			}
			if flags&TrunFirstSampleFlags != 0 {
				_ = rd.ReadUint32() // skip
			}

			atom := &Atom{Name: name}

			for i := uint32(0); i < samples; i++ {
				if flags&TrunSampleDuration != 0 {
					atom.SamplesDuration = append(atom.SamplesDuration, rd.ReadUint32())
				}
				if flags&TrunSampleSize != 0 {
					atom.SamplesSize = append(atom.SamplesSize, rd.ReadUint32())
				}
				if flags&TrunSampleFlags != 0 {
					_ = rd.ReadUint32() // skip
				}
				if flags&TrunSampleCTS != 0 {
					_ = rd.ReadUint32() // skip
				}
			}

			if rd.EOF {
				return nil, io.EOF
			}

			atoms = append(atoms, atom)

		case Mdat:
			atoms = append(atoms, &Atom{Name: name, Data: data})

		default:
			println("iso: unsupported atom: " + name)
		}
	}

	return atoms, nil
}
