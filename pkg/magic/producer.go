package magic

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/magic/bitstream"
	"github.com/AlexxIT/go2rtc/pkg/magic/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/mpjpeg"
	"github.com/AlexxIT/go2rtc/pkg/wav"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
)

func Open(r io.Reader) (core.Producer, error) {
	rd := core.NewReadBuffer(r)

	b, err := rd.Peek(4)
	if err != nil {
		return nil, err
	}

	switch string(b) {
	case annexb.StartCode:
		return bitstream.Open(rd)
	case wav.FourCC:
		return wav.Open(rd)
	case y4m.FourCC:
		return y4m.Open(rd)
	}

	switch string(b[:3]) {
	case flv.Signature:
		return flv.Open(rd)
	}

	switch string(b[:2]) {
	case "\xFF\xD8":
		return mjpeg.Open(rd)
	case "\xFF\xF1", "\xFF\xF9":
		return aac.Open(rd)
	case "--":
		return mpjpeg.Open(rd)
	}

	switch b[0] {
	case mpegts.SyncByte:
		return mpegts.Open(rd)
	}

	// support MJPEG with trash on start
	// https://github.com/AlexxIT/go2rtc/issues/747
	if b, err = rd.Peek(4096); err != nil {
		return nil, err
	}

	if i := bytes.Index(b, []byte{0xFF, 0xD8, 0xFF, 0xDB}); i > 0 {
		_, _ = io.ReadFull(rd, make([]byte, i))
		return mjpeg.Open(rd)
	}

	return nil, errors.New("magic: unsupported header: " + hex.EncodeToString(b[:4]))
}
