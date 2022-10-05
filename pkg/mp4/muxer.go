package mp4

import (
	"encoding/binary"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/deepch/vdk/format/fmp4/fmp4io"
	"github.com/deepch/vdk/format/mp4/mp4io"
	"github.com/deepch/vdk/format/mp4f/mp4fio"
	"github.com/pion/rtp"
)

type Muxer struct {
	fragIndex uint32
	dts       uint64
	pts       uint32
	data      []byte
	total     int
}

func (m *Muxer) MimeType(codecs []*streamer.Codec) string {
	s := `video/mp4; codecs="`

	for _, codec := range codecs {
		switch codec.Name {
		case streamer.CodecH264:
			s += "avc1." + h264.GetProfileLevelID(codec.FmtpLine)
		case streamer.CodecH265:
			// +Safari +Chrome +Edge -iOS15 -Android13
			s += "hvc1.1.6.L93.B0" // hev1.1.6.L93.B0
		}
	}

	return s + `"`
}

func (m *Muxer) GetInit(codecs []*streamer.Codec) ([]byte, error) {
	moov := MOOV()

	for _, codec := range codecs {
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

			trak := TRAK()
			trak.Media.Header.TimeScale = int32(codec.ClockRate)
			trak.Header.TrackWidth = float64(width)
			trak.Header.TrackHeight = float64(height)

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

			trak.Media.Handler = &mp4io.HandlerRefer{
				SubType: [4]byte{'v', 'i', 'd', 'e'},
				Name:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'm', 'a', 'i', 'n', 0},
			}

			moov.Tracks = append(moov.Tracks, trak)

		case streamer.CodecH265:
			vps, sps, pps := h265.GetParameterSet(codec.FmtpLine)
			if sps == nil {
				return nil, fmt.Errorf("empty SPS: %#v", codec)
			}

			codecData, err := h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps)
			if err != nil {
				return nil, err
			}

			width := codecData.Width()
			height := codecData.Height()

			trak := TRAK()
			trak.Media.Header.TimeScale = int32(codec.ClockRate)
			trak.Header.TrackWidth = float64(width)
			trak.Header.TrackHeight = float64(height)

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

			trak.Media.Handler = &mp4io.HandlerRefer{
				SubType: [4]byte{'v', 'i', 'd', 'e'},
				Name:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'm', 'a', 'i', 'n', 0},
			}

			moov.Tracks = append(moov.Tracks, trak)
		}
	}

	data := make([]byte, moov.Len())
	moov.Marshal(data)

	return append(FTYP(), data...), nil
}

func (m *Muxer) Rewind() {
	m.dts = 0
	m.pts = 0
}

func (m *Muxer) Marshal(packet *rtp.Packet) []byte {
	trackID := uint8(1)

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
					Data: []byte{0x00, 0x02, 0x00, 0x20, 0x00, 0x00, 0x00, trackID, 0x01, 0x01, 0x00, 0x00},
				},
				DecodeTime: &mp4fio.TrackFragDecodeTime{
					Version: 1,
					Flags:   0,
					Time:    m.dts,
				},
				Run: run,
			},
		},
	}

	entry := mp4io.TrackFragRunEntry{
		//Duration: 90000,
		Size: uint32(len(packet.Payload)),
	}

	newTime := packet.Timestamp
	if m.pts > 0 {
		//m.dts += uint64(newTime - m.pts)
		entry.Duration = newTime - m.pts
		m.dts += uint64(entry.Duration)
	}
	m.pts = newTime

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

	m.total += moofLen + mdatLen

	return buf
}
