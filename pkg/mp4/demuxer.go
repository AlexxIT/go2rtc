package mp4

import (
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/pion/rtp"
)

type Demuxer struct {
	codecs     map[uint32]*core.Codec
	timeScales map[uint32]float32
}

func (d *Demuxer) Probe(init []byte) (medias []*core.Media) {
	var trackID, timeScale uint32

	if d.codecs == nil {
		d.codecs = make(map[uint32]*core.Codec)
		d.timeScales = make(map[uint32]float32)
	}

	atoms, _ := iso.DecodeAtoms(init)
	for _, atom := range atoms {
		var codec *core.Codec

		switch atom := atom.(type) {
		case *iso.AtomTkhd:
			trackID = atom.TrackID
		case *iso.AtomMdhd:
			timeScale = atom.TimeScale
		case *iso.AtomVideo:
			switch atom.Name {
			case "avc1":
				codec = h264.ConfigToCodec(atom.Config)
			}
		case *iso.AtomAudio:
			switch atom.Name {
			case "mp4a":
				codec = aac.ConfigToCodec(atom.Config)
			}
		}

		if codec != nil {
			d.codecs[trackID] = codec
			d.timeScales[trackID] = float32(codec.ClockRate) / float32(timeScale)

			medias = append(medias, &core.Media{
				Kind:      codec.Kind(),
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			})
		}
	}

	return
}

func (d *Demuxer) GetTrackID(codec *core.Codec) uint32 {
	for trackID, c := range d.codecs {
		if c == codec {
			return trackID
		}
	}
	return 0
}

func (d *Demuxer) Demux(data2 []byte) (trackID uint32, packets []*core.Packet) {
	atoms, err := iso.DecodeAtoms(data2)
	if err != nil {
		return 0, nil
	}

	var ts uint32
	var trun *iso.AtomTrun
	var data []byte

	for _, atom := range atoms {
		switch atom := atom.(type) {
		case *iso.AtomTfhd:
			trackID = atom.TrackID
		case *iso.AtomTfdt:
			ts = uint32(atom.DecodeTime)
		case *iso.AtomTrun:
			trun = atom
		case *iso.AtomMdat:
			data = atom.Data
		}
	}

	timeScale := d.timeScales[trackID]
	if timeScale == 0 {
		return 0, nil
	}

	n := len(trun.SamplesDuration)
	packets = make([]*core.Packet, n)

	for i := 0; i < n; i++ {
		duration := trun.SamplesDuration[i]
		size := trun.SamplesSize[i]

		// can be SPS, PPS and IFrame in one packet
		timestamp := uint32(float32(ts) * timeScale)
		packets[i] = &rtp.Packet{
			Header:  rtp.Header{Timestamp: timestamp},
			Payload: data[:size],
		}

		data = data[size:]
		ts += duration
	}

	return
}
