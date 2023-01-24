package httpflv

import (
	"bufio"
	"bytes"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
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

	if flags&flvio.FILE_HAS_VIDEO != 0 {
		c.videoIdx = -1
	}

	if flags&flvio.FILE_HAS_AUDIO != 0 {
		c.audioIdx = -1
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

	videoIdx int8
	audioIdx int8
}

func (c *Conn) Streams() ([]av.CodecData, error) {
	var video, audio av.CodecData

	// Normal software sends:
	// 1. Video/audio flag in header
	// 2. MetaData as first tag (with video/audio codec info)
	// 3. Video/audio headers in 2nd and 3rd tag

	// Reolink camera sends:
	// 1. Empty video/audio flag
	// 2. MedaData without stereo key for AAC
	// 3. Audio header after Video keyframe tag

	waitVideo := c.videoIdx != 0
	waitAudio := c.audioIdx != 0

	for i := 0; i < 20; i++ {
		tag, _, err := flvio.ReadTag(c.reader, c.buf)
		if err != nil {
			return nil, err
		}

		//log.Printf("[FLV] type=%d avc=%d aac=%d video=%t audio=%t", tag.Type, tag.AVCPacketType, tag.AACPacketType, video != nil, audio != nil)

		switch tag.Type {
		case flvio.TAG_SCRIPTDATA:
			if meta := NewReader(tag.Data).ReadMetaData(); meta != nil {
				waitVideo = meta["videocodecid"] != nil

				// don't wait audio tag because parse all info from MetaData
				waitAudio = false

				audio = parseAudioConfig(meta)
			} else {
				waitVideo = bytes.Contains(tag.Data, []byte("videocodecid"))
				waitAudio = bytes.Contains(tag.Data, []byte("audiocodecid"))
			}

		case flvio.TAG_VIDEO:
			if tag.AVCPacketType == flvio.AVC_SEQHDR {
				video, _ = h264parser.NewCodecDataFromAVCDecoderConfRecord(tag.Data)
			}
			waitVideo = false

		case flvio.TAG_AUDIO:
			if tag.SoundFormat == flvio.SOUND_AAC && tag.AACPacketType == flvio.AAC_SEQHDR {
				audio, _ = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(tag.Data)
			}
			waitAudio = false
		}

		if !waitVideo && !waitAudio {
			break
		}
	}

	if video != nil && audio != nil {
		c.videoIdx = 0
		c.audioIdx = 1
		return []av.CodecData{video, audio}, nil
	} else if video != nil {
		c.videoIdx = 0
		c.audioIdx = -1
		return []av.CodecData{video}, nil
	} else if audio != nil {
		c.videoIdx = -1
		c.audioIdx = 0
		return []av.CodecData{audio}, nil
	}

	return nil, nil
}

func (c *Conn) ReadPacket() (av.Packet, error) {
	for {
		tag, ts, err := ReadTag(c.reader, c.buf)
		if err != nil {
			return av.Packet{}, err
		}

		switch tag.Type {
		case flvio.TAG_VIDEO:
			if c.videoIdx < 0 || tag.AVCPacketType != flvio.AVC_NALU {
				continue
			}

			//log.Printf("[FLV] %v, len: %d, ts: %10d", h264.Types(tag.Data), len(tag.Data), flvio.TsToTime(ts))

			return av.Packet{
				Idx:             c.videoIdx,
				Data:            tag.Data,
				CompositionTime: flvio.TsToTime(tag.CompositionTime),
				IsKeyFrame:      tag.FrameType == flvio.FRAME_KEY,
				Time:            flvio.TsToTime(ts),
			}, nil

		case flvio.TAG_AUDIO:
			if c.audioIdx < 0 || tag.SoundFormat != flvio.SOUND_AAC || tag.AACPacketType != flvio.AAC_RAW {
				continue
			}

			return av.Packet{Idx: c.audioIdx, Data: tag.Data, Time: flvio.TsToTime(ts)}, nil
		}
	}
}

func (c *Conn) Close() (err error) {
	return c.conn.Close()
}

func parseAudioConfig(meta map[string]interface{}) av.CodecData {
	if meta["audiocodecid"] != float64(10) {
		return nil
	}

	config := aacparser.MPEG4AudioConfig{
		ObjectType: aacparser.AOT_AAC_LC,
	}

	switch v := meta["audiosamplerate"].(type) {
	case float64:
		config.SampleRate = int(v)
	default:
		return nil
	}

	switch meta["stereo"] {
	case true:
		config.ChannelConfig = 2
		config.ChannelLayout = av.CH_STEREO
	default:
		// Reolink doesn't have this setting
		config.ChannelConfig = 1
		config.ChannelLayout = av.CH_MONO
	}

	buf := &bytes.Buffer{}
	if err := aacparser.WriteMPEG4AudioConfig(buf, config); err != nil {
		return nil
	}

	return aacparser.CodecData{
		Config:      config,
		ConfigBytes: buf.Bytes(),
	}
}
