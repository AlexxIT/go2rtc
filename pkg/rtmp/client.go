package rtmp

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/rtmpt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtmp"
	"github.com/pion/rtp"
	"strings"
	"time"
)

// Conn for RTMP and RTMPT (flv over HTTP)
type Conn interface {
	Streams() (streams []av.CodecData, err error)
	ReadPacket() (pkt av.Packet, err error)
	Close() (err error)
}

type Client struct {
	streamer.Element

	URI string

	medias []*streamer.Media
	tracks []*streamer.Track

	conn   Conn
	closed bool

	receive int
}

func NewClient(uri string) *Client {
	return &Client{URI: uri}
}

func (c *Client) Dial() (err error) {
	if strings.HasPrefix(c.URI, "http") {
		c.conn, err = rtmpt.Dial(c.URI)
	} else {
		c.conn, err = rtmp.Dial(c.URI)
	}

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
			info := stream.(h264parser.CodecData).RecordInfo

			fmtp := fmt.Sprintf(
				"profile-level-id=%02X%02X%02X;sprop-parameter-sets=%s,%s",
				info.AVCProfileIndication, info.ProfileCompatibility, info.AVCLevelIndication,
				base64.StdEncoding.EncodeToString(info.SPS[0]),
				base64.StdEncoding.EncodeToString(info.PPS[0]),
			)

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
			// TODO: fix support
			cd := stream.(aacparser.CodecData)

			// a=fmtp:97 streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1588
			fmtp := fmt.Sprintf(
				"config=%s",
				hex.EncodeToString(cd.ConfigBytes),
			)

			codec := &streamer.Codec{
				Name:      streamer.CodecAAC,
				ClockRate: uint32(cd.Config.SampleRate),
				Channels:  uint16(cd.Config.ChannelConfig),
				FmtpLine:  fmtp,
			}

			media := &streamer.Media{
				Kind:      streamer.KindAudio,
				Direction: streamer.DirectionSendonly,
				Codecs:    []*streamer.Codec{codec},
			}
			c.medias = append(c.medias, media)

			track := &streamer.Track{
				Codec: codec, Direction: media.Direction,
			}
			c.tracks = append(c.tracks, track)

		default:
			fmt.Printf("[rtmp] unsupported codec %+v\n", stream)
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

		// convert seconds to RTP timestamp
		timestamp := uint32(pkt.Time * time.Duration(track.Codec.ClockRate) / time.Second)

		packet := &rtp.Packet{
			Header:  rtp.Header{Timestamp: timestamp},
			Payload: pkt.Data,
		}
		_ = track.WriteRTP(packet)
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}
