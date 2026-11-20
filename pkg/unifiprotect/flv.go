package unifiprotect

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
)

const (
	TagAudio = 8
	TagVideo = 9
	TagOpus  = 10
	TagData  = 18

	maxTagSize     = 4 * 1024 * 1024
	maxTrailerScan = 1 << 16
)

var ErrFraming = errors.New("unifi-protect: invalid extended FLV framing")

type Tag struct {
	Type      byte
	Timestamp uint32
	Data      []byte
}

type Metadata struct {
	StreamName     string
	AudioFrequency uint32
	AudioChannels  uint8
	VideoWidth     uint16
	VideoHeight    uint16
}

// Reader removes the private trailer UniFi appends to each extended-FLV tag.
// The next tag is located structurally because the trailer size varies with
// the payload. One complete tag is held while its successor is validated.
type Reader struct {
	r   io.Reader
	buf []byte
	eof bool

	headerDone bool
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) ReadTag() (*Tag, error) {
	if err := r.readHeader(); err != nil {
		return nil, err
	}

	for skipped := 0; ; skipped++ {
		if err := r.fill(15); err != nil {
			return nil, err
		}

		if !validTagType(r.buf[0]) {
			if skipped >= maxTrailerScan {
				return nil, ErrFraming
			}
			r.buf = r.buf[1:]
			continue
		}

		dataSize := int(uint24(r.buf[1:4]))
		if dataSize > maxTagSize {
			if skipped >= maxTrailerScan {
				return nil, ErrFraming
			}
			r.buf = r.buf[1:]
			continue
		}

		tagSize := 11 + dataSize
		if err := r.fill(tagSize + 4); err != nil {
			return nil, err
		}
		if binary.BigEndian.Uint32(r.buf[tagSize:tagSize+4]) != uint32(tagSize) {
			if skipped >= maxTrailerScan {
				return nil, ErrFraming
			}
			r.buf = r.buf[1:]
			continue
		}

		tag := &Tag{
			Type:      r.buf[0],
			Timestamp: uint24(r.buf[4:7]) | uint32(r.buf[7])<<24,
			Data:      append([]byte(nil), r.buf[11:tagSize]...),
		}

		next, err := r.nextTag(tagSize + 4)
		if err != nil {
			return nil, err
		}
		r.buf = r.buf[next:]
		return tag, nil
	}
}

func (r *Reader) readHeader() error {
	if r.headerDone {
		return nil
	}

	if err := r.fill(3); err != nil {
		return err
	}
	if string(r.buf[:3]) != "FLV" {
		r.headerDone = true
		return nil
	}

	if err := r.fill(9); err != nil {
		return err
	}
	offset := int(binary.BigEndian.Uint32(r.buf[5:9]))
	if offset < 9 || offset > maxTagSize {
		return fmt.Errorf("%w: invalid FLV header size %d", ErrFraming, offset)
	}
	if err := r.fill(offset + 4); err != nil {
		return err
	}
	if binary.BigEndian.Uint32(r.buf[offset:offset+4]) != 0 {
		return fmt.Errorf("%w: invalid first previous-tag size", ErrFraming)
	}
	r.buf = r.buf[offset+4:]
	r.headerDone = true
	return nil
}

func (r *Reader) nextTag(tagEnd int) (int, error) {
	for {
		pending := false
		scanMax := len(r.buf) - tagEnd
		if scanMax > maxTrailerScan {
			scanMax = maxTrailerScan
		}

		for gap := 0; gap <= scanMax; gap++ {
			pos := tagEnd + gap
			if pos+11 > len(r.buf) {
				pending = true
				break
			}
			if !validTagType(r.buf[pos]) || r.buf[pos+8] != 0 || r.buf[pos+9] != 0 || r.buf[pos+10] != 0 {
				continue
			}

			dataSize := int(uint24(r.buf[pos+1 : pos+4]))
			if dataSize > maxTagSize {
				continue
			}
			nextEnd := pos + 11 + dataSize
			if nextEnd+4 > len(r.buf) {
				pending = true
				continue
			}
			if binary.BigEndian.Uint32(r.buf[nextEnd:nextEnd+4]) == uint32(11+dataSize) {
				return pos, nil
			}
		}

		if r.eof {
			// The final tag has no successor to validate. Its own framing was
			// already checked, so emit it and discard any final private trailer.
			return len(r.buf), nil
		}
		if len(r.buf) > tagEnd+maxTrailerScan+11+maxTagSize+4 {
			return 0, fmt.Errorf("%w: next tag not found", ErrFraming)
		}

		before := len(r.buf)
		if err := r.readMore(); err != nil {
			if errors.Is(err, io.EOF) {
				continue
			}
			return 0, err
		}
		if !pending && len(r.buf) == before {
			return 0, io.ErrNoProgress
		}
	}
}

func (r *Reader) fill(size int) error {
	for len(r.buf) < size {
		if r.eof {
			return io.EOF
		}
		if err := r.readMore(); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
	return nil
}

func (r *Reader) readMore() error {
	b := make([]byte, 32*1024)
	n, err := r.r.Read(b)
	if n > 0 {
		r.buf = append(r.buf, b[:n]...)
	}
	if errors.Is(err, io.EOF) {
		r.eof = true
	}
	if n > 0 {
		return nil
	}
	if err == nil {
		return io.ErrNoProgress
	}
	return err
}

func ParseMetadata(tag *Tag) (Metadata, bool, error) {
	if tag.Type != TagData {
		return Metadata{}, false, nil
	}

	items, err := amf.NewReader(tag.Data).ReadItems()
	if err != nil {
		return Metadata{}, false, err
	}

	for i, item := range items {
		name, ok := item.(string)
		if !ok || name != "onMetaData" || i+1 >= len(items) {
			continue
		}
		obj, ok := items[i+1].(map[string]any)
		if !ok {
			return Metadata{}, false, errors.New("unifi-protect: invalid onMetaData object")
		}
		return Metadata{
			StreamName:     stringValue(obj["streamName"]),
			AudioFrequency: uint32(numberValue(obj["audioFrequency"])),
			AudioChannels:  uint8(numberValue(obj["audioChannels"])),
			VideoWidth:     uint16(numberValue(obj["videoWidth"])),
			VideoHeight:    uint16(numberValue(obj["videoHeight"])),
		}, true, nil
	}

	return Metadata{}, false, nil
}

func validTagType(v byte) bool {
	return v == TagAudio || v == TagVideo || v == TagOpus || v == TagData
}

func uint24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) float64 {
	n, _ := v.(float64)
	return n
}
