package mp4

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/pion/rtp"
)

type Demuxer struct {
	codecs     map[uint32]*core.Codec
	timeScales map[uint32]float32
}

type TrackPackets struct {
	TrackID uint32
	Packets []*core.Packet
}

type TrackData struct {
	DecodeTime uint32
	Trun       *iso.AtomTrun
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
			case "hvc1", "hev1":
				codec = h265.ConfigToCodec(atom.Config)
			}
		case *iso.AtomAudio:
			switch atom.Name {
			case "mp4a":
				// G.711 PCMU audio detection for 8kHz mono (Tuya...)
				if atom.SampleRate == 8000 && atom.Channels == 1 {
					codec = &core.Codec{
						Name:        core.CodecPCMU,
						ClockRate:   8000,
						Channels:    1,
						PayloadType: 0,
					}
				} else {
					codec = aac.ConfigToCodec(atom.Config)
				}
			}
		}

		if codec != nil {
			d.codecs[trackID] = codec
			d.timeScales[trackID] = float32(codec.ClockRate) / float32(timeScale)

			medias = append(medias, &core.Media{
				ID:        fmt.Sprintf("trackID=%d", trackID),
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

// DemuxAll returns packets from all tracks found in the fragment
func (d *Demuxer) DemuxAll(data []byte) []TrackPackets {
	atoms, err := iso.DecodeAtoms(data)
	if err != nil {
		return nil
	}

	// Map to store track-specific data
	trackData := make(map[uint32]TrackData)
	var mdat []byte

	// First pass: collect all track data
	for _, atom := range atoms {
		switch atom := atom.(type) {
		case *iso.AtomMdat:
			mdat = atom.Data
		}
	}

	// Temporary variables to track current track ID while parsing
	var currentTrackID uint32

	// Second pass: process traf boxes
	for _, atom := range atoms {
		switch atom := atom.(type) {
		case *iso.AtomTfhd:
			currentTrackID = atom.TrackID

			// Initialize track data if not exists
			if _, ok := trackData[currentTrackID]; !ok {
				trackData[currentTrackID] = TrackData{}
			}

		case *iso.AtomTfdt:
			if currentTrackID != 0 {
				td := trackData[currentTrackID]
				td.DecodeTime = uint32(atom.DecodeTime)
				trackData[currentTrackID] = td
			}

		case *iso.AtomTrun:
			if currentTrackID != 0 {
				td := trackData[currentTrackID]
				td.Trun = atom
				trackData[currentTrackID] = td
			}
		}
	}

	// Process all tracks and collect results
	var results []TrackPackets

	for tid, td := range trackData {
		if td.Trun == nil || mdat == nil || len(td.Trun.SamplesSize) == 0 {
			continue
		}

		codec := d.codecs[tid]
		if codec == nil {
			continue
		}

		timeScale := d.timeScales[tid]

		var packets []*core.Packet
		switch codec.Kind() {
		case "video":
			packets = createVideoPackets(td.Trun, mdat, td.DecodeTime, timeScale)
		case "audio":
			packets = createAudioPackets(td.Trun, mdat, td.DecodeTime, timeScale, codec)
		}

		if len(packets) > 0 {
			results = append(results, TrackPackets{
				TrackID: tid,
				Packets: packets,
			})
		}
	}

	return results
}

// Creates video packets (H.264/H.265)
func createVideoPackets(trun *iso.AtomTrun, mdat []byte, decodeTime uint32, timeScale float32) []*core.Packet {
	n := len(trun.SamplesSize)
	hasDurations := len(trun.SamplesDuration) > 0

	packets := make([]*core.Packet, n)
	offset := uint32(0)
	ts := decodeTime

	for i := 0; i < n; i++ {
		// Get duration from array or use default
		var duration uint32
		if hasDurations && i < len(trun.SamplesDuration) {
			duration = trun.SamplesDuration[i]
		} else {
			duration = 1000 // Default for video
		}

		size := trun.SamplesSize[i]

		if offset+size > uint32(len(mdat)) {
			return packets[:i]
		}

		timestamp := uint32(float32(ts) * timeScale)
		packets[i] = &rtp.Packet{
			Header:  rtp.Header{Timestamp: timestamp},
			Payload: mdat[offset : offset+size],
		}

		offset += size
		ts += duration
	}

	return packets
}

// Creates audio packets (G.711, AAC, etc.)
func createAudioPackets(trun *iso.AtomTrun, mdat []byte, decodeTime uint32, timeScale float32, codec *core.Codec) []*core.Packet {
	n := len(trun.SamplesSize)
	hasDurations := len(trun.SamplesDuration) > 0

	packets := make([]*core.Packet, n)
	offset := uint32(0)
	ts := decodeTime
	isPCM := codec.Name == core.CodecPCMU || codec.Name == core.CodecPCMA || codec.Name == core.CodecPCM || codec.Name == core.CodecPCML

	for i := 0; i < n; i++ {
		size := trun.SamplesSize[i]

		// Calculate duration based on codec
		var duration uint32
		if hasDurations && i < len(trun.SamplesDuration) {
			duration = trun.SamplesDuration[i]
		} else if isPCM {
			duration = size
		} else {
			duration = 1024
		}

		if offset+size > uint32(len(mdat)) {
			return packets[:i]
		}

		// Calculate timestamp based on codec
		var timestamp uint32
		if isPCM {
			timestamp = ts
		} else {
			timestamp = uint32(float32(ts) * timeScale)
		}

		packets[i] = &rtp.Packet{
			Header:  rtp.Header{Timestamp: timestamp},
			Payload: mdat[offset : offset+size],
		}

		offset += size
		ts += duration
	}

	return packets
}
