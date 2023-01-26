package mp4

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/deepch/vdk/format/fmp4/fmp4io"
	"github.com/deepch/vdk/format/mp4/mp4io"
	"github.com/deepch/vdk/format/mp4f/mp4fio"
	"github.com/pion/rtp"
)

type Muxer struct {
	fragIndex uint32
	dts       []uint64
	pts       []uint32
}

func (m *Muxer) MimeType(codecs []*streamer.Codec) string {
	s := `video/mp4; codecs="`

	for i, codec := range codecs {
		if i > 0 {
			s += ","
		}

		switch codec.Name {
		case streamer.CodecH264:
			s += "avc1." + h264.GetProfileLevelID(codec.FmtpLine)
		case streamer.CodecH265:
			// H.265 profile=main level=5.1
			// hvc1 - supported in Safari, hev1 - doesn't, both supported in Chrome
			s += "hvc1.1.6.L153.B0"
		case streamer.CodecAAC:
			s += "mp4a.40.2"
		}
	}

	return s + `"`
}

func (m *Muxer) GetInit(codecs []*streamer.Codec) ([]byte, error) {
	moov := MOOV()

	for i, codec := range codecs {
		switch codec.Name {
		case streamer.CodecH264:
			sps, pps := h264.GetParameterSet(codec.FmtpLine)
			if sps == nil {
				// some dummy SPS and PPS not a problem
				sps = []byte{0x67, 0x42, 0x00, 0x0a, 0xf8, 0x41, 0xa2}
				pps = []byte{0x68, 0xce, 0x38, 0x80}
			}

			codecData, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
			if err != nil {
				return nil, err
			}

			width := codecData.Width()
			height := codecData.Height()

			trak := TRAK(i + 1)
			trak.Header.TrackWidth = float64(width)
			trak.Header.TrackHeight = float64(height)
			trak.Media.Header.TimeScale = int32(codec.ClockRate)
			trak.Media.Handler = &mp4io.HandlerRefer{
				SubType: [4]byte{'v', 'i', 'd', 'e'},
				Name:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'm', 'a', 'i', 'n', 0},
			}
			trak.Media.Info.Video = &mp4io.VideoMediaInfo{
				Flags: 0x000001,
			}
			trak.Media.Info.Sample.SampleDesc.AVC1Desc = &mp4io.AVC1Desc{
				DataRefIdx:           1,
				HorizontalResolution: 72,
				VorizontalResolution: 72,
				Width:                int16(width),
				Height:               int16(height),
				FrameCount:           1,
				Depth:                24,
				ColorTableId:         -1,
				Conf: &mp4io.AVC1Conf{
					Data: codecData.AVCDecoderConfRecordBytes(),
				},
			}

			moov.Tracks = append(moov.Tracks, trak)

		case streamer.CodecH265:
			vps, sps, pps := h265.GetParameterSet(codec.FmtpLine)
			if sps == nil {
				// some dummy SPS and PPS not a problem
				vps = []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x99, 0xac, 0x09}
				sps = []byte{0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x99, 0xa0, 0x01, 0x40, 0x20, 0x05, 0xa1, 0xfe, 0x5a, 0xee, 0x46, 0xc1, 0xae, 0x55, 0x04}
				pps = []byte{0x44, 0x01, 0xc0, 0x73, 0xc0, 0x4c, 0x90}
			}

			codecData, err := h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps)
			if err != nil {
				return nil, err
			}

			width := codecData.Width()
			height := codecData.Height()

			trak := TRAK(i + 1)
			trak.Header.TrackWidth = float64(width)
			trak.Header.TrackHeight = float64(height)
			trak.Media.Header.TimeScale = int32(codec.ClockRate)
			trak.Media.Handler = &mp4io.HandlerRefer{
				SubType: [4]byte{'v', 'i', 'd', 'e'},
				Name:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'm', 'a', 'i', 'n', 0},
			}
			trak.Media.Info.Video = &mp4io.VideoMediaInfo{
				Flags: 0x000001,
			}
			trak.Media.Info.Sample.SampleDesc.HV1Desc = &mp4io.HV1Desc{
				DataRefIdx:           1,
				HorizontalResolution: 72,
				VorizontalResolution: 72,
				Width:                int16(width),
				Height:               int16(height),
				FrameCount:           1,
				Depth:                24,
				ColorTableId:         -1,
				Conf: &mp4io.HV1Conf{
					Data: codecData.AVCDecoderConfRecordBytes(),
				},
			}

			moov.Tracks = append(moov.Tracks, trak)

		case streamer.CodecAAC:
			s := streamer.Between(codec.FmtpLine, "config=", ";")
			b, err := hex.DecodeString(s)
			if err != nil {
				return nil, err
			}

			trak := TRAK(i + 1)
			trak.Header.AlternateGroup = 1
			trak.Header.Duration = 0
			trak.Header.Volume = 1
			trak.Media.Header.TimeScale = int32(codec.ClockRate)

			trak.Media.Handler = &mp4io.HandlerRefer{
				SubType: [4]byte{'s', 'o', 'u', 'n'},
				Name:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'm', 'a', 'i', 'n', 0},
			}
			trak.Media.Info.Sound = &mp4io.SoundMediaInfo{}

			trak.Media.Info.Sample.SampleDesc.MP4ADesc = &mp4io.MP4ADesc{
				DataRefIdx:       1,
				NumberOfChannels: int16(codec.Channels),
				SampleSize:       int16(av.FLTP.BytesPerSample() * 4),
				SampleRate:       float64(codec.ClockRate),
				Unknowns:         []mp4io.Atom{ESDS(b)},
			}

			moov.Tracks = append(moov.Tracks, trak)
		}

		trex := &mp4io.TrackExtend{
			TrackId:               uint32(i + 1),
			DefaultSampleDescIdx:  1,
			DefaultSampleDuration: 0,
		}
		moov.MovieExtend.Tracks = append(moov.MovieExtend.Tracks, trex)

		m.pts = append(m.pts, 0)
		m.dts = append(m.dts, 0)
	}

	data := make([]byte, moov.Len())
	moov.Marshal(data)

	return append(FTYP(), data...), nil
}

func (m *Muxer) Reset() {
	m.fragIndex = 0
	for i := range m.dts {
		m.dts[i] = 0
		m.pts[i] = 0
	}
}

func (m *Muxer) Marshal(trackID byte, packet *rtp.Packet) []byte {
	run := &mp4fio.TrackFragRun{
		Flags:            0x000b05,
		FirstSampleFlags: uint32(fmp4io.SampleNoDependencies),
		DataOffset:       0,
		Entries:          []mp4io.TrackFragRunEntry{},
	}

	moof := &mp4fio.MovieFrag{
		Header: &mp4fio.MovieFragHeader{
			Seqnum: m.fragIndex + 1,
		},
		Tracks: []*mp4fio.TrackFrag{
			{
				Header: &mp4fio.TrackFragHeader{
					Data: []byte{0x00, 0x02, 0x00, 0x20, 0x00, 0x00, 0x00, trackID + 1, 0x01, 0x01, 0x00, 0x00},
				},
				DecodeTime: &mp4fio.TrackFragDecodeTime{
					Version: 1,
					Flags:   0,
					Time:    m.dts[trackID],
				},
				Run: run,
			},
		},
	}

	entry := mp4io.TrackFragRunEntry{
		Size: uint32(len(packet.Payload)),
	}

	newTime := packet.Timestamp
	if m.pts[trackID] > 0 {
		entry.Duration = newTime - m.pts[trackID]
		m.dts[trackID] += uint64(entry.Duration)
	} else {
		// important, or Safari will fail with first frame
		entry.Duration = 1
	}
	m.pts[trackID] = newTime

	// important before moof.Len()
	run.Entries = append(run.Entries, entry)

	moofLen := moof.Len()
	mdatLen := 8 + len(packet.Payload)

	// important after moof.Len()
	run.DataOffset = uint32(moofLen + 8)

	buf := make([]byte, moofLen+mdatLen)
	moof.Marshal(buf)

	binary.BigEndian.PutUint32(buf[moofLen:], uint32(mdatLen))
	copy(buf[moofLen+4:], "mdat")
	copy(buf[moofLen+8:], packet.Payload)

	m.fragIndex++

	//m.total += moofLen + mdatLen

	return buf
}
