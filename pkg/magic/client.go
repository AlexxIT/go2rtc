package magic

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/magic/bitstream"
	"github.com/AlexxIT/go2rtc/pkg/magic/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

// Client - can read unknown bytestream and autodetect format
type Client struct {
	rd   *core.ReadSeeker
	prod core.Producer
}

func Open(r io.Reader) (*Client, error) {
	rd := core.NewReadSeeker(r)

	b, err := rd.Peek(4)
	if err != nil {
		return nil, err
	}

	switch {
	case bytes.HasPrefix(b, []byte(annexb.StartCode)) || bytes.HasPrefix(b, []byte{0, 0, 1}):
		var prod core.Producer
		if prod, err = bitstream.Open(rd); err != nil {
			return nil, err
		}
		return &Client{rd: rd, prod: prod}, nil

	case bytes.HasPrefix(b, []byte{0xFF, 0xD8}):
		return &Client{rd: rd, prod: mjpeg.NewClient(rd)}, nil

	case bytes.HasPrefix(b, []byte{'F', 'L', 'V'}):
		break // TODO

	case b[0] == mpegts.SyncByte:
		break // TODO
	}

	return nil, errors.New("magic: unsupported header: " + hex.EncodeToString(b))
}
