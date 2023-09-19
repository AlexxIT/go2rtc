package dvrip

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Client struct {
	core.Listener

	uri     string
	conn    net.Conn
	reader  *bufio.Reader
	session uint32
	seq     uint32
	stream  string

	medias     []*core.Media
	receivers  []*core.Receiver
	videoTrack *core.Receiver
	audioTrack *core.Receiver

	videoTS  uint32
	videoDT  uint32
	audioTS  uint32
	audioSeq uint16

	recv uint32
}

type Response map[string]any

const Login = uint16(1000)
const OPMonitorClaim = uint16(1413)
const OPMonitorStart = uint16(1410)

func NewClient(url string) *Client {
	return &Client{uri: url}
}

func (c *Client) Dial() (err error) {
	u, err := url.Parse(c.uri)
	if err != nil {
		return
	}

	if u.Port() == "" {
		// add default TCP port
		u.Host += ":34567"
	}

	c.conn, err = net.DialTimeout("tcp", u.Host, time.Second*3)
	if err != nil {
		return
	}

	c.reader = bufio.NewReader(c.conn)

	query := u.Query()
	channel := query.Get("channel")
	if channel == "" {
		channel = "0"
	}

	subtype := query.Get("subtype")
	switch subtype {
	case "", "0":
		subtype = "Main"
	case "1":
		subtype = "Extra1"
	}

	c.stream = fmt.Sprintf(
		`{"Channel":%s,"CombinMode":"NONE","StreamType":"%s","TransMode":"TCP"}`,
		channel, subtype,
	)

	if u.User != nil {
		pass, _ := u.User.Password()
		return c.Login(u.User.Username(), pass)
	} else {
		return c.Login("admin", "admin")
	}
}

func (c *Client) Login(user, pass string) (err error) {
	data := fmt.Sprintf(
		`{"EncryptType":"MD5","LoginType":"DVRIP-Web","PassWord":"%s","UserName":"%s"}`,
		SofiaHash(pass), user,
	)

	if err = c.Request(Login, data); err != nil {
		return
	}

	_, err = c.ResponseJSON()
	return
}

func (c *Client) Play() (err error) {
	format := `{"Name":"OPMonitor","SessionID":"0x%08X","OPMonitor":{"Action":"%s","Parameter":%s}}`

	data := fmt.Sprintf(format, c.session, "Claim", c.stream)
	if err = c.Request(OPMonitorClaim, data); err != nil {
		return
	}
	if _, err = c.ResponseJSON(); err != nil {
		return
	}

	data = fmt.Sprintf(format, c.session, "Start", c.stream)
	return c.Request(OPMonitorStart, data)
}

func (c *Client) Handle() error {
	var buf []byte
	var size int

	var probe byte
	if c.medias == nil {
		probe = 1
	}

	for {
		b, err := c.Response()
		if err != nil {
			return err
		}

		// collect data from multiple packets
		if size > 0 {
			buf = append(buf, b...)
			if len(buf) < size {
				continue
			}
			if len(buf) > size {
				return errors.New("wrong size")
			}
			b = buf
		}

		dataType := binary.BigEndian.Uint32(b)
		switch dataType {
		case 0x1FC, 0x1FE:
			size = int(binary.LittleEndian.Uint32(b[12:])) + 16
		case 0x1FD: // PFrame
			size = int(binary.LittleEndian.Uint32(b[4:])) + 8
		case 0x1FA, 0x1F9:
			size = int(binary.LittleEndian.Uint16(b[6:])) + 8
		default:
			return fmt.Errorf("unknown type: %X", dataType)
		}

		if len(b) < size {
			buf = b
			continue // need to collect data from next packets
		}

		//log.Printf("[DVR] type: %d, len: %d", dataType, len(b))

		switch dataType {
		case 0x1FC, 0x1FE: // video IFrame
			payload := annexb.EncodeToAVCC(b[16:], false)

			if c.videoTrack == nil {
				fps := b[5]
				//width := uint16(b[6]) * 8
				//height := uint16(b[7]) * 8
				//println(width, height)
				ts := b[8:]

				// the exact value of the start TS does not matter
				c.videoTS = binary.LittleEndian.Uint32(ts)
				c.videoDT = 90000 / uint32(fps)

				c.AddVideoTrack(b[4], payload)
			}

			if c.videoTrack != nil {
				c.videoTS += c.videoDT

				packet := &rtp.Packet{
					Header:  rtp.Header{Timestamp: c.videoTS},
					Payload: payload,
				}

				//log.Printf("[AVC] %v, len: %d, ts: %10d", h265.Types(payload), len(payload), packet.Timestamp)

				c.videoTrack.WriteRTP(packet)
			}

		case 0x1FD: // PFrame
			if c.videoTrack != nil {
				c.videoTS += c.videoDT

				packet := &rtp.Packet{
					Header:  rtp.Header{Timestamp: c.videoTS},
					Payload: annexb.EncodeToAVCC(b[8:], false),
				}

				//log.Printf("[DVR] %v, len: %d, ts: %10d", h265.Types(packet.Payload), len(packet.Payload), packet.Timestamp)

				c.videoTrack.WriteRTP(packet)
			}

		case 0x1FA, 0x1F9: // audio
			if c.audioTrack == nil {
				// the exact value of the start TS does not matter
				c.audioTS = c.videoTS

				c.AddAudioTrack(b[4], b[5])
			}

			if c.audioTrack != nil {
				for b != nil {
					payload := b[8:size]
					if len(b) > size {
						b = b[size:]
					} else {
						b = nil
					}

					c.audioTS += uint32(len(payload))
					c.audioSeq++

					packet := &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							SequenceNumber: c.audioSeq,
							Timestamp:      c.audioTS,
						},
						Payload: payload,
					}

					//log.Printf("[DVR] len: %d, ts: %10d", len(packet.Payload), packet.Timestamp)

					c.audioTrack.WriteRTP(packet)
				}
			}
		}

		if probe != 0 {
			probe++
			if (c.videoTS > 0 && c.audioTS > 0) || probe == 20 {
				return nil
			}
		}

		size = 0
	}
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Request(cmd uint16, data string) (err error) {
	b := make([]byte, 20, 128)
	b[0] = 255
	binary.LittleEndian.PutUint32(b[4:], c.session)
	binary.LittleEndian.PutUint32(b[8:], c.seq)
	binary.LittleEndian.PutUint16(b[14:], cmd)
	binary.LittleEndian.PutUint32(b[16:], uint32(len(data))+2)
	b = append(b, data...)
	b = append(b, 0x0A, 0x00)

	c.seq++

	if err = c.conn.SetWriteDeadline(time.Now().Add(time.Second * 5)); err != nil {
		return
	}

	_, err = c.conn.Write(b)
	return
}

func (c *Client) Response() (b []byte, err error) {
	if err = c.conn.SetReadDeadline(time.Now().Add(time.Second * 5)); err != nil {
		return
	}

	b = make([]byte, 20)
	if _, err = io.ReadFull(c.reader, b); err != nil {
		return
	}

	c.recv += 20

	if b[0] != 255 {
		return nil, errors.New("read error")
	}

	c.session = binary.LittleEndian.Uint32(b[4:])
	size := binary.LittleEndian.Uint32(b[16:])

	b = make([]byte, size)
	if _, err = io.ReadFull(c.reader, b); err != nil {
		return
	}

	c.recv += size

	return
}

func (c *Client) ResponseJSON() (res Response, err error) {
	b, err := c.Response()
	if err != nil {
		return
	}

	res = Response{}
	if err = json.Unmarshal(b[:len(b)-2], &res); err != nil {
		return
	}

	if v, ok := res["Ret"].(float64); !ok || (v != 100 && v != 515) {
		err = fmt.Errorf("wrong response: %s", b)
	}
	return
}

func (c *Client) AddVideoTrack(mediaCode byte, payload []byte) {
	var codec *core.Codec
	switch mediaCode {
	case 0x02, 0x12:
		codec = &core.Codec{
			Name:        core.CodecH264,
			ClockRate:   90000,
			PayloadType: core.PayloadTypeRAW,
			FmtpLine:    h264.GetFmtpLine(payload),
		}

	case 0x03, 0x13, 0x43, 0x53:
		codec = &core.Codec{
			Name:        core.CodecH265,
			ClockRate:   90000,
			PayloadType: core.PayloadTypeRAW,
			FmtpLine:    "profile-id=1",
		}

		for {
			size := 4 + int(binary.BigEndian.Uint32(payload))

			switch h265.NALUType(payload) {
			case h265.NALUTypeVPS:
				codec.FmtpLine += ";sprop-vps=" + base64.StdEncoding.EncodeToString(payload[4:size])
			case h265.NALUTypeSPS:
				codec.FmtpLine += ";sprop-sps=" + base64.StdEncoding.EncodeToString(payload[4:size])
			case h265.NALUTypePPS:
				codec.FmtpLine += ";sprop-pps=" + base64.StdEncoding.EncodeToString(payload[4:size])
			}

			if size < len(payload) {
				payload = payload[size:]
			} else {
				break
			}
		}
	default:
		println("[DVRIP] unsupported video codec:", mediaCode)
		return
	}

	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codec},
	}
	c.medias = append(c.medias, media)

	c.videoTrack = core.NewReceiver(media, codec)
	c.receivers = append(c.receivers, c.videoTrack)
}

var sampleRates = []uint32{4000, 8000, 11025, 16000, 20000, 22050, 32000, 44100, 48000}

func (c *Client) AddAudioTrack(mediaCode byte, sampleRate byte) {
	// https://github.com/vigoss30611/buildroot-ltc/blob/master/system/qm/ipc/ProtocolService/src/ZhiNuo/inc/zn_dh_base_type.h
	// PCM8 = 7, G729, IMA_ADPCM, G711U, G721, PCM8_VWIS, MS_ADPCM, G711A, PCM16
	var codec *core.Codec
	switch mediaCode {
	case 10: // G711U
		codec = &core.Codec{
			Name: core.CodecPCMU,
		}
	case 14: // G711A
		codec = &core.Codec{
			Name: core.CodecPCMA,
		}
	default:
		println("[DVRIP] unsupported audio codec:", mediaCode)
		return
	}

	if sampleRate <= byte(len(sampleRates)) {
		codec.ClockRate = sampleRates[sampleRate-1]
	}

	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codec},
	}
	c.medias = append(c.medias, media)

	c.audioTrack = core.NewReceiver(media, codec)
	c.receivers = append(c.receivers, c.audioTrack)
}

func SofiaHash(password string) string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	sofia := make([]byte, 0, 8)
	hash := md5.Sum([]byte(password))
	for i := 0; i < md5.Size; i += 2 {
		j := uint16(hash[i]) + uint16(hash[i+1])
		sofia = append(sofia, chars[j%62])
	}

	return string(sofia)
}
