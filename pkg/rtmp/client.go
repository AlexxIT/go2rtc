package rtmp

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/httpflv"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtmp"
	"github.com/pion/rtp"
	"net/http"
	"time"
)

// Conn for RTMP and RTMPT (flv over HTTP)
type Conn interface {
	Streams() (streams []av.CodecData, err error)
	ReadPacket() (pkt av.Packet, err error)
	Close() (err error)
}

type Client struct {
	core.Listener

	URI string

	medias    []*core.Media
	receivers []*core.Receiver

	conn   Conn
	closed bool

	recv int
}

func NewClient(uri string) *Client {
	return &Client{URI: uri}
}

func (c *Client) Dial() (err error) {
	c.conn, err = rtmp.Dial(c.URI)
	return
}

// Accept - convert http.Response to Client
func Accept(res *http.Response) (*Client, error) {
	conn, err := httpflv.Accept(res)
	if err != nil {
		return nil, err
	}
	return &Client{URI: res.Request.URL.String(), conn: conn}, nil
}

func (c *Client) Describe() (err error) {
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

			codec := &core.Codec{
				Name:        core.CodecH264,
				ClockRate:   90000,
				FmtpLine:    fmtp,
				PayloadType: core.PayloadTypeRAW,
			}

			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)

			track := core.NewReceiver(media, codec)
			c.receivers = append(c.receivers, track)

		case av.AAC:
			// TODO: fix support
			cd := stream.(aacparser.CodecData)

			codec := &core.Codec{
				Name:      core.CodecAAC,
				ClockRate: uint32(cd.Config.SampleRate),
				Channels:  uint16(cd.Config.ChannelConfig),
				//  a=fmtp:97 streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1588
				FmtpLine:    "streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=" + hex.EncodeToString(cd.ConfigBytes),
				PayloadType: core.PayloadTypeRAW,
			}

			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)

			track := core.NewReceiver(media, codec)
			c.receivers = append(c.receivers, track)

		default:
			fmt.Printf("[rtmp] unsupported codec %+v\n", stream)
		}
	}

	return
}

func (c *Client) Handle() (err error) {
	for {
		var pkt av.Packet
		pkt, err = c.conn.ReadPacket()
		if err != nil {
			if c.closed {
				return nil
			}
			return
		}

		c.recv += len(pkt.Data)

		track := c.receivers[int(pkt.Idx)]

		// convert seconds to RTP timestamp
		timestamp := uint32(pkt.Time * time.Duration(track.Codec.ClockRate) / time.Second)

		packet := &rtp.Packet{
			Header:  rtp.Header{Timestamp: timestamp},
			Payload: pkt.Data,
		}
		track.WriteRTP(packet)
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}
