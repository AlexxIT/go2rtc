package magic

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/magic/bitstream"
	"github.com/AlexxIT/go2rtc/pkg/magic/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/multipart"
)

func Open(r io.Reader) (core.Producer, error) {
	rd := core.NewReadBuffer(r)

	b, err := rd.Peek(4)
	if err != nil {
		return nil, err
	}

	switch {
	case bytes.HasPrefix(b, []byte(annexb.StartCode)):
		return bitstream.Open(rd)

	case bytes.HasPrefix(b, []byte{0xFF, 0xD8}):
		return mjpeg.Open(rd)

	case bytes.HasPrefix(b, []byte(flv.Signature)):
		return flv.Open(rd)

	case bytes.HasPrefix(b, []byte("--")):
		return multipart.Open(rd)

	case b[0] == mpegts.SyncByte:
		return mpegts.Open(rd)
	}

	return nil, errors.New("magic: unsupported header: " + hex.EncodeToString(b))
}
