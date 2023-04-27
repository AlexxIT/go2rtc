package mp4

import (
	"encoding/hex"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/pion/rtp"
)

type Muxer struct {
	fragIndex uint32
	dts       []uint64
	pts       []uint32
	codecs    []*core.Codec
}

const (
	MimeH264 = "avc1.640029"
	MimeH265 = "hvc1.1.6.L153.B0"
	MimeAAC  = "mp4a.40.2"
	MimeFlac = "flac"
	MimeOpus = "opus"
)

func (m *Muxer) MimeCodecs(codecs []*core.Codec) string {
	var s string

	for i, codec := range codecs {
		if i > 0 {
			s += ","
		}

		switch codec.Name {
		case core.CodecH264:
			s += "avc1." + h264.GetProfileLevelID(codec.FmtpLine)
		case core.CodecH265:
			// H.265 profile=main level=5.1
			// hvc1 - supported in Safari, hev1 - doesn't, both supported in Chrome
			s += MimeH265
		case core.CodecAAC:
			s += MimeAAC
		case core.CodecOpus:
			s += MimeOpus
		case core.CodecFLAC:
			s += MimeFlac
		}
	}

	return s
}

func (m *Muxer) GetInit(codecs []*core.Codec) ([]byte, error) {
	mv := iso.NewMovie(1024)
	mv.WriteFileType()

	mv.StartAtom(iso.Moov)
	mv.WriteMovieHeader()

	for i, codec := range codecs {
		switch codec.Name {
		case core.CodecH264:
			sps, pps := h264.GetParameterSet(codec.FmtpLine)
			// some dummy SPS and PPS not a problem
			if len(sps) == 0 {
				sps = []byte{0x67, 0x42, 0x00, 0x0a, 0xf8, 0x41, 0xa2}
			}
			if len(pps) == 0 {
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

		case core.CodecH265:
			vps, sps, pps := h265.GetParameterSet(codec.FmtpLine)
			// some dummy SPS and PPS not a problem
			if len(vps) == 0 {
				vps = []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x99, 0xac, 0x09}
			}
			if len(sps) == 0 {
				sps = []byte{0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x99, 0xa0, 0x01, 0x40, 0x20, 0x05, 0xa1, 0xfe, 0x5a, 0xee, 0x46, 0xc1, 0xae, 0x55, 0x04}
			}
			if len(pps) == 0 {
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

		case core.CodecAAC:
			s := core.Between(codec.FmtpLine, "config=", ";")
			b, err := hex.DecodeString(s)
			if err != nil {
				return nil, err
			}

			mv.WriteAudioTrack(
				uint32(i+1), codec.Name, codec.ClockRate, codec.Channels, b,
			)

		case core.CodecOpus, core.CodecMP3, core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecFLAC:
			mv.WriteAudioTrack(
				uint32(i+1), codec.Name, codec.ClockRate, codec.Channels, nil,
			)
		}

		m.dts = append(m.dts, 0)
		m.pts = append(m.pts, 0)
		m.codecs = append(m.codecs, codec)
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
	codec := m.codecs[trackID]

	duration := packet.Timestamp - m.pts[trackID]
	m.pts[trackID] = packet.Timestamp

	// minumum duration important for MSE in Apple Safari
	if duration == 0 || duration > codec.ClockRate {
		duration = codec.ClockRate/1000 + 1
		m.pts[trackID] += duration
	}

	size := len(packet.Payload)

	// flags important for Apple Finder video preview
	var flags uint32
	switch codec.Name {
	case core.CodecH264:
		if h264.IsKeyframe(packet.Payload) {
			flags = iso.SampleVideoIFrame
		} else {
			flags = iso.SampleVideoNonIFrame
		}
	case core.CodecH265:
		if h265.IsKeyframe(packet.Payload) {
			flags = iso.SampleVideoIFrame
		} else {
			flags = iso.SampleVideoNonIFrame
		}
	default:
		flags = iso.SampleAudio // not important
	}

	m.fragIndex++

	mv := iso.NewMovie(1024 + size)
	mv.WriteMovieFragment(
		m.fragIndex, uint32(trackID+1), duration, uint32(size), flags, m.dts[trackID],
	)
	mv.WriteData(packet.Payload)

	//log.Printf("[MP4] track=%d ts=%6d dur=%5d idx=%3d len=%d", trackID+1, m.dts[trackID], duration, m.fragIndex, len(packet.Payload))

	m.dts[trackID] += uint64(duration)

	return mv.Bytes()
}
