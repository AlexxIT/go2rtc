package mp4

import (
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/mp4"
	"time"
)

func MarshalMP4(sps, pps, frame []byte) []byte {
	writer := &MemoryWriter{}
	muxer := mp4.NewMuxer(writer)

	stream, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
	if err != nil {
		panic(err)
	}

	if err = muxer.WriteHeader([]av.CodecData{stream}); err != nil {
		panic(err)
	}

	pkt := av.Packet{
		CompositionTime: time.Millisecond,
		IsKeyFrame:      true,
		Duration:        time.Second,
		Data:            frame,
	}
	if err = muxer.WritePacket(pkt); err != nil {
		panic(err)
	}
	if err = muxer.WriteTrailer(); err != nil {
		panic(err)
	}

	return writer.buf
}
