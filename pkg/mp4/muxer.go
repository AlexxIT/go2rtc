package mp4

import (
	"encoding/hex"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/pion/rtp"
)

type Muxer struct {
	fragIndex uint32
	dts       []uint64
	pts       []uint32
}

const (
	MimeH264 = "avc1.640029"
	MimeH265 = "hvc1.1.6.L153.B0"
	MimeAAC  = "mp4a.40.2"
	MimeOpus = "opus"
)

func (m *Muxer) MimeCodecs(codecs []*streamer.Codec) string {
	var s string

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
			s += MimeH265
		case streamer.CodecAAC:
			s += MimeAAC
		case streamer.CodecOpus:
			s += MimeOpus
		}
	}

	return s
}

func (m *Muxer) GetInit(codecs []*streamer.Codec) ([]byte, error) {
	mv := iso.NewMovie(1024)
	mv.WriteFileType()

	mv.StartAtom(iso.Moov)
	mv.WriteMovieHeader()

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

			mv.WriteVideoTrack(
				uint32(i+1), codec.Name, codec.ClockRate,
				uint16(codecData.Width()), uint16(codecData.Height()),
				codecData.AVCDecoderConfRecordBytes(),
			)

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

			mv.WriteVideoTrack(
				uint32(i+1), codec.Name, codec.ClockRate,
				uint16(codecData.Width()), uint16(codecData.Height()),
				codecData.AVCDecoderConfRecordBytes(),
			)

		case streamer.CodecAAC:
			s := streamer.Between(codec.FmtpLine, "config=", ";")
			b, err := hex.DecodeString(s)
			if err != nil {
				return nil, err
			}

			mv.WriteAudioTrack(
				uint32(i+1), codec.Name, codec.ClockRate, codec.Channels, b,
			)

		case streamer.CodecOpus, streamer.CodecMP3, streamer.CodecPCMU, streamer.CodecPCMA:
			mv.WriteAudioTrack(
				uint32(i+1), codec.Name, codec.ClockRate, codec.Channels, nil,
			)
		}

		m.pts = append(m.pts, 0)
		m.dts = append(m.dts, 0)
	}

	mv.StartAtom(iso.MoovMvex)
	for i := range codecs {
		mv.WriteTrackExtend(uint32(i + 1))
	}
	mv.EndAtom() // MVEX

	mv.EndAtom() // MOOV

	return mv.Bytes(), nil
}

func (m *Muxer) Reset() {
	m.fragIndex = 0
	for i := range m.dts {
		m.dts[i] = 0
		m.pts[i] = 0
	}
}

func (m *Muxer) Marshal(trackID byte, packet *rtp.Packet) []byte {
	// important before increment
	time := m.dts[trackID]

	m.fragIndex++

	var duration uint32
	newTime := packet.Timestamp
	if m.pts[trackID] > 0 {
		duration = newTime - m.pts[trackID]
		m.dts[trackID] += uint64(duration)
	} else {
		// important, or Safari will fail with first frame
		duration = 1
	}
	m.pts[trackID] = newTime

	mv := iso.NewMovie(1024 + len(packet.Payload))
	mv.WriteMovieFragment(
		m.fragIndex, uint32(trackID+1), duration,
		uint32(len(packet.Payload)), time,
	)
	mv.WriteData(packet.Payload)

	return mv.Bytes()
}
