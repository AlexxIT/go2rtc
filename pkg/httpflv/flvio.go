package httpflv

import (
	"fmt"
	"github.com/deepch/vdk/format/flv/flvio"
	"github.com/deepch/vdk/utils/bits/pio"
	"io"
)

// TODO: rewrite all of this someday

func ReadTag(r io.Reader, b []byte) (tag flvio.Tag, ts int32, err error) {
	if _, err = io.ReadFull(r, b[:flvio.TagHeaderLength]); err != nil {
		return
	}
	var datalen int
	if tag, ts, datalen, err = flvio.ParseTagHeader(b); err != nil {
		return
	}

	data := make([]byte, datalen)
	if _, err = io.ReadFull(r, data); err != nil {
		return
	}

	n, err := ParseHeader(&tag, data)
	if err != nil {
		return
	}
	tag.Data = data[n:]

	if _, err = io.ReadFull(r, b[:4]); err != nil {
		return
	}
	return
}

func ParseHeader(self *flvio.Tag, b []byte) (n int, err error) {
	switch self.Type {
	case flvio.TAG_AUDIO:
		return audioParseHeader(self, b)

	case flvio.TAG_VIDEO:
		return videoParseHeader(self, b)
	}

	return
}

func audioParseHeader(tag *flvio.Tag, b []byte) (n int, err error) {
	if len(b) < n+1 {
		err = fmt.Errorf("audiodata: parse invalid")
		return
	}

	flags := b[n]
	n++
	tag.SoundFormat = flags >> 4
	tag.SoundRate = (flags >> 2) & 0x3
	tag.SoundSize = (flags >> 1) & 0x1
	tag.SoundType = flags & 0x1

	switch tag.SoundFormat {
	case flvio.SOUND_AAC:
		if len(b) < n+1 {
			err = fmt.Errorf("audiodata: parse invalid")
			return
		}
		tag.AACPacketType = b[n]
		n++
	}

	return
}

func videoParseHeader(tag *flvio.Tag, b []byte) (n int, err error) {
	if len(b) < n+1 {
		err = fmt.Errorf("videodata: parse invalid")
		return
	}
	flags := b[n]
	tag.FrameType = flags >> 4
	tag.CodecID = flags & 0xf
	n++

	if len(b) < n+4 {
		err = fmt.Errorf("videodata: parse invalid")
		return
	}
	tag.AVCPacketType = b[n]
	n++

	tag.CompositionTime = pio.I24BE(b[n:])
	n += 3

	return
}
