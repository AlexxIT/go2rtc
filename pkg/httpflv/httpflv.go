package httpflv

import (
	"bufio"
	"errors"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/flv/flvio"
	"github.com/deepch/vdk/utils/bits/pio"
	"io"
	"net/http"
)

func Dial(uri string) (*Conn, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return Accept(res)
}

func Accept(res *http.Response) (*Conn, error) {
	c := Conn{
		conn:   res.Body,
		reader: bufio.NewReaderSize(res.Body, pio.RecommendBufioSize),
		buf:    make([]byte, 256),
	}

	if _, err := io.ReadFull(c.reader, c.buf[:flvio.FileHeaderLength]); err != nil {
		return nil, err
	}

	flags, n, err := flvio.ParseFileHeader(c.buf)
	if err != nil {
		return nil, err
	}

	if flags&flvio.FILE_HAS_VIDEO == 0 {
		return nil, errors.New("not supported")
	}

	if _, err = c.reader.Discard(n); err != nil {
		return nil, err
	}

	return &c, nil
}

type Conn struct {
	conn   io.ReadCloser
	reader *bufio.Reader
	buf    []byte
}

func (c *Conn) Streams() ([]av.CodecData, error) {
	for {
		tag, _, err := flvio.ReadTag(c.reader, c.buf)
		if err != nil {
			return nil, err
		}

		if tag.Type != flvio.TAG_VIDEO || tag.AVCPacketType != flvio.AAC_SEQHDR {
			continue
		}

		stream, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(tag.Data)
		if err != nil {
			return nil, err
		}

		return []av.CodecData{stream}, nil
	}
}

func (c *Conn) ReadPacket() (av.Packet, error) {
	for {
		tag, ts, err := flvio.ReadTag(c.reader, c.buf)
		if err != nil {
			return av.Packet{}, err
		}

		if tag.Type != flvio.TAG_VIDEO || tag.AVCPacketType != flvio.AVC_NALU {
			continue
		}

		return av.Packet{
			Idx:             0,
			Data:            tag.Data,
			CompositionTime: flvio.TsToTime(tag.CompositionTime),
			IsKeyFrame:      tag.FrameType == flvio.FRAME_KEY,
			Time:            flvio.TsToTime(ts),
		}, nil
	}
}

func (c *Conn) Close() (err error) {
	return c.conn.Close()
}
