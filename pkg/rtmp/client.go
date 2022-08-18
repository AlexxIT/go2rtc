package rtmp

import (
	"encoding/base64"
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtmp"
	"github.com/pion/rtp"
	"time"
)

type Client struct {
	streamer.Element

	URI string

	medias []*streamer.Media
	tracks []*streamer.Track

	conn   *rtmp.Conn
	closed bool

	receive int
}

func NewClient(uri string) *Client {
	return &Client{URI: uri}
}

func (c *Client) Dial() (err error) {
	c.conn, err = rtmp.Dial(c.URI)
	if err != nil {
		return
	}

	// important to get SPS/PPS
	streams, err := c.conn.Streams()
	if err != nil {
		return
	}

	for _, stream := range streams {
		switch stream.Type() {
		case av.H264:
			cd := stream.(h264parser.CodecData)
			fmtp := "sprop-parameter-sets=" +
				base64.StdEncoding.EncodeToString(cd.RecordInfo.SPS[0]) + "," +
				base64.StdEncoding.EncodeToString(cd.RecordInfo.PPS[0])

			codec := &streamer.Codec{
				Name:        streamer.CodecH264,
				ClockRate:   90000,
				FmtpLine:    fmtp,
				PayloadType: h264.PayloadTypeAVC,
			}

			media := &streamer.Media{
				Kind:      streamer.KindVideo,
				Direction: streamer.DirectionSendonly,
				Codecs:    []*streamer.Codec{codec},
			}
			c.medias = append(c.medias, media)

			track := &streamer.Track{
				Codec: codec, Direction: media.Direction,
			}
			c.tracks = append(c.tracks, track)

		case av.AAC:
			panic("not implemented")
		default:
			panic("unsupported codec")
		}
	}

	c.Fire(streamer.StateReady)

	return
}

func (c *Client) Handle() (err error) {
	defer c.Fire(streamer.StateNull)

	c.Fire(streamer.StatePlaying)

	for {
		var pkt av.Packet
		pkt, err = c.conn.ReadPacket()
		if err != nil {
			if c.closed {
				return nil
			}
			return
		}

		c.receive += len(pkt.Data)

		track := c.tracks[int(pkt.Idx)]

		timestamp := uint32(pkt.Time / time.Duration(track.Codec.ClockRate))

		var payloads [][]byte
		if track.Codec.Name == streamer.CodecH264 {
			payloads = splitAVC(pkt.Data)
		} else {
			payloads = [][]byte{pkt.Data}
		}

		for _, payload := range payloads {
			packet := &rtp.Packet{
				Header:  rtp.Header{Timestamp: timestamp},
				Payload: payload,
			}
			_ = track.WriteRTP(packet)
		}
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func splitAVC(data []byte) [][]byte {
	var nals [][]byte
	for {
		// get AVC length
		size := int(binary.BigEndian.Uint32(data))

		// check if multiple items in one packet
		if size+4 < len(data) {
			nals = append(nals, data[:size+4])
			data = data[size+4:]
		} else {
			nals = append(nals, data)
			break
		}
	}
	return nals
}
