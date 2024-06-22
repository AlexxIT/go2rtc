package dvrip

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection

	client *Client

	video, audio *core.Receiver

	videoTS  uint32
	videoDT  uint32
	audioTS  uint32
	audioSeq uint16
}

func (c *Producer) Start() error {
	for {
		pType, b, err := c.client.ReadPacket()
		if err != nil {
			return err
		}

		//log.Printf("[DVR] type: %d, len: %d", dataType, len(b))

		switch pType {
		case 0xFC, 0xFE, 0xFD:
			if c.video == nil {
				continue
			}

			var payload []byte
			if pType != 0xFD {
				payload = b[16:] // iframe
			} else {
				payload = b[8:] // pframe
			}

			c.videoTS += c.videoDT

			packet := &rtp.Packet{
				Header:  rtp.Header{Timestamp: c.videoTS},
				Payload: annexb.EncodeToAVCC(payload, false),
			}

			//log.Printf("[AVC] %v, len: %d, ts: %10d", h265.Types(payload), len(payload), packet.Timestamp)

			c.video.WriteRTP(packet)

		case 0xFA: // audio
			if c.audio == nil {
				continue
			}

			payload := b[8:]

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

			c.audio.WriteRTP(packet)

		case 0xF9: // unknown

		default:
			println(fmt.Sprintf("dvrip: unknown packet type: %d", pType))
		}
	}
}

func (c *Producer) probe() error {
	if err := c.client.Play(); err != nil {
		return err
	}

	rd := core.NewReadBuffer(c.client.rd)
	rd.BufferSize = core.ProbeSize
	defer func() {
		c.client.buf = nil
		rd.Reset()
	}()

	c.client.rd = rd

	// some awful cameras has VERY rare keyframes
	// so we wait video+audio for default probe time
	// and wait anything for 15 seconds
	timeoutBoth := time.Now().Add(core.ProbeTimeout)
	timeoutAny := time.Now().Add(time.Second * 15)

	for {
		if now := time.Now(); now.Before(timeoutBoth) {
			if c.video != nil && c.audio != nil {
				return nil
			}
		} else if now.Before(timeoutAny) {
			if c.video != nil || c.audio != nil {
				return nil
			}
		} else {
			return errors.New("dvrip: can't probe medias")
		}

		tag, b, err := c.client.ReadPacket()
		if err != nil {
			return err
		}

		switch tag {
		case 0xFC, 0xFE: // video
			if c.video != nil {
				continue
			}

			fps := b[5]
			//width := uint16(b[6]) * 8
			//height := uint16(b[7]) * 8
			//println(width, height)
			ts := b[8:]

			// the exact value of the start TS does not matter
			c.videoTS = binary.LittleEndian.Uint32(ts)
			c.videoDT = 90000 / uint32(fps)

			payload := annexb.EncodeToAVCC(b[16:], false)
			c.addVideoTrack(b[4], payload)

		case 0xFA: // audio
			if c.audio != nil {
				continue
			}

			// the exact value of the start TS does not matter
			c.audioTS = c.videoTS

			c.addAudioTrack(b[4], b[5])
		}
	}
}

func (c *Producer) addVideoTrack(mediaCode byte, payload []byte) {
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
	c.Medias = append(c.Medias, media)

	c.video = core.NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, c.video)
}

var sampleRates = []uint32{4000, 8000, 11025, 16000, 20000, 22050, 32000, 44100, 48000}

func (c *Producer) addAudioTrack(mediaCode byte, sampleRate byte) {
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
	c.Medias = append(c.Medias, media)

	c.audio = core.NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, c.audio)
}

//func (c *Client) MarshalJSON() ([]byte, error) {
//	info := &core.Info{
//		Type:       "DVRIP active producer",
//		RemoteAddr: c.conn.RemoteAddr().String(),
//		Medias:     c.Medias,
//		Receivers:  c.Receivers,
//		Recv:       c.Recv,
//	}
//	return json.Marshal(info)
//}
