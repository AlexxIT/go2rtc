package legacy

import (
	"encoding/binary"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

func Dial(rawURL string) (*Producer, error) {
	client, err := NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	err = client.StartMedia(query.Get("subtype"), "")
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	medias, err := probe(client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	c := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/legacy",
			Protocol:   "tutk+udp",
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client: client,
	}
	return c, nil
}

type Producer struct {
	core.Connection
	client *Client
}

const codecXiaobaiPCMA = 1 // chuangmi.camera.xiaobai

func probe(client *Client) ([]*core.Media, error) {
	tutk.DebugLog("=== probe: starting for model %s ===", client.model)

	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	_ = client.WriteCommandJSON(0x0704, `{}`)

	var vcodec, acodec *core.Codec
	var videoFound time.Time

	for {
		hdr, payload, err := client.ReadPacket()
		if err != nil {
			tutk.DebugLog("probe ReadPacket error: %v, vcodec=%v, acodec=%v", err, vcodec != nil, acodec != nil)
			if vcodec != nil {
				tutk.DebugLog("probe: Proceeding with video only")
				break
			}
			return nil, err
		}

		first16 := payload
		if len(first16) > 16 {
			first16 = first16[:16]
		}
		tutk.DebugLog("probe Packet: hdr0=0x%02x, len=%d, first16=%x", hdr[0], len(payload), first16)

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			if vcodec == nil {
				avcc := annexb.EncodeToAVCC(payload)
				if codec == tutk.CodecH264 {
					for b := avcc; len(b) > 4; {
						size := 4 + int(binary.BigEndian.Uint32(b))
						if h264.NALUType(b) == h264.NALUTypeSPS {
							vcodec = h264.AVCCToCodec(b)
							break
						}
						if size < len(b) {
							b = b[size:]
						} else {
							break
						}
					}
					if vcodec == nil {
						vcodec = &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
					}
				} else {
					for b := avcc; len(b) > 4; {
						size := 4 + int(binary.BigEndian.Uint32(b))
						if h265.NALUType(b) == h265.NALUTypeVPS {
							vcodec = h265.AVCCToCodec(b)
							break
						}
						if size < len(b) {
							b = b[size:]
						} else {
							break
						}
					}
					if vcodec == nil {
						vcodec = &core.Codec{
							Name:        core.CodecH265,
							ClockRate:   90000,
							FmtpLine:    "profile-id=1;sprop-vps=QAEMAf//AUAAAAMAAAMAAAMAAAMAmawJ;sprop-sps=QgEBAUAAAAMAAAMAAAMAAAMAmaAFAgGB8blruRsGuVQQ;sprop-pps=RAHAc8BBgwk=",
							PayloadType: core.PayloadTypeRAW,
						}
						tutk.DebugLog("probe: Using camera H265 profile fallback")
					}
				}
				if vcodec != nil {
					tutk.DebugLog("probe: Detected video codec %s (fmtp=%s)", vcodec.Name, vcodec.FmtpLine)
				}
			}
		case tutk.CodecPCMA, codecXiaobaiPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
				tutk.DebugLog("probe: Detected audio codec PCMA")
			}
		case tutk.CodecPCML:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
				tutk.DebugLog("probe: Detected audio codec PCML")
			}
		case tutk.CodecAACLATM:
			if acodec == nil {
				acodec = aac.ADTSToCodec(payload)
				if acodec != nil {
					acodec.PayloadType = core.PayloadTypeRAW
					tutk.DebugLog("probe: Detected audio codec AAC")
				}
			}
		}

		if vcodec != nil && acodec != nil {
			tutk.DebugLog("probe: Both vcodec and acodec found! Probe successful.")
			break
		}
		if vcodec != nil {
			if videoFound.IsZero() {
				videoFound = time.Now()
			} else if time.Since(videoFound) > 3*time.Second {
				tutk.DebugLog("probe: Video found, 3s passed without audio: proceeding with video only!")
				break
			}
		}
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}
	if acodec != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		})

		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{acodec.Clone()},
		})
	}
	return medias, nil
}

func (c *Producer) Protocol() string {
	return "tutk+udp"
}

func (c *Producer) Start() error {
	var audioTS, videoTS uint32
	var videoSeq, audioSeq uint16

	var ps []byte
	for _, media := range c.Medias {
		if media.Kind == core.KindVideo && len(media.Codecs) > 0 {
			vps, sps, pps := h265.GetParameterSet(media.Codecs[0].FmtpLine)
			ps = h264.JoinNALU(vps, sps, pps)
			tutk.DebugLog("Producer Start: initialized ps len=%d", len(ps))
			break
		}
	}

	_ = c.client.WriteCommandJSON(0x0704, `{}`)

	for {
		_ = c.client.SetDeadline(time.Now().Add(5 * time.Second))
		hdr, payload, err := c.client.ReadPacket()
		if err != nil {
			return err
		}

		n := len(payload)
		c.Recv += n

		// TODO: rewrite this
		var name string
		var pkt *core.Packet

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			avcc := annexb.EncodeToAVCC(payload)
			if len(avcc) < 5 {
				continue
			}

			typesBefore := h265.Types(avcc)
			isKey := (codec == tutk.CodecH265 && h265.IsKeyframe(avcc)) || (len(hdr) >= 3 && hdr[2] == 1)
			if isKey {
				tutk.DebugLog("c.Start: isKey=TRUE! len=%d typesBefore=%v", len(payload), typesBefore)
			}
			if codec == tutk.CodecH265 && len(ps) > 0 && (isKey || videoSeq == 0) {
				if h265.NALUType(avcc) != h265.NALUTypeVPS {
					avcc = h264.Join(ps, avcc)
				}
			}
			if videoSeq > 0 && videoSeq%100 == 0 {
				_ = c.client.WriteCommandJSON(0x0704, `{}`)
			}

			if videoTS == 0 {
				videoTS = core.Now90000()
			} else {
				videoTS += 4500 // exactly 20 fps (90000 / 20)
			}

			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: videoSeq,
					Timestamp:      videoTS,
				},
				Payload: avcc,
			}
			videoSeq++

			if codec == tutk.CodecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
				types := h265.Types(avcc)
				if h265.IsKeyframe(avcc) {
					tutk.DebugLog("c.Start: >>>>> KEYFRAME RECEIVED! len=%d types=%v <<<<<", len(payload), types)
				} else if videoSeq <= 20 || videoSeq%50 == 0 {
					tutk.DebugLog("c.Start H265 #%d: len=%d isKey=false types=%v", videoSeq, len(payload), types)
				}
			}

		case tutk.CodecPCMA, tutk.CodecPCML, codecXiaobaiPCMA:
			pkt = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			switch codec {
			case tutk.CodecPCMA, codecXiaobaiPCMA:
				name = core.CodecPCMA
				audioTS += uint32(n)
			case tutk.CodecPCML:
				name = core.CodecPCML
				audioTS += uint32(n / 2) // because 16bit
			}

		case tutk.CodecAACLATM:
			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			name = core.CodecAAC
			audioTS += 1024
		}

		for _, recv := range c.Receivers {
			if recv.Codec.Name == name {
				recv.WriteRTP(pkt)
				break
			}
		}
	}
}

func (c *Producer) Stop() error {
	_ = c.client.StopMedia()
	return c.Connection.Stop()
}
