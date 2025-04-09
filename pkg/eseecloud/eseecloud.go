package eseecloud

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *core.ReadBuffer

	videoPT, audioPT uint8
}

func Dial(rawURL string) (core.Producer, error) {
	rawURL, _ = strings.CutPrefix(rawURL, "eseecloud")
	res, err := http.Get("http" + rawURL)
	if err != nil {
		return nil, err
	}

	prod, err := Open(res.Body)
	if err != nil {
		return nil, err
	}

	if info, ok := prod.(core.Info); ok {
		info.SetProtocol("http")
		info.SetURL(rawURL)
	}

	return prod, nil
}

func Open(r io.Reader) (core.Producer, error) {
	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "eseecloud",
			Transport:  r,
		},
		rd: core.NewReadBuffer(r),
	}

	if err := prod.probe(); err != nil {
		return nil, err
	}

	return prod, nil
}

func (p *Producer) probe() error {
	b, err := p.rd.Peek(1024)
	if err != nil {
		return err
	}

	i := bytes.Index(b, []byte("\r\n\r\n"))
	if i == -1 {
		return io.EOF
	}

	b = make([]byte, i+4)
	_, _ = p.rd.Read(b)

	re := regexp.MustCompile(`m=(video|audio) (\d+) (\w+)/(\d+)\S*`)
	for _, item := range re.FindAllStringSubmatch(string(b), 2) {
		p.SDP += item[0] + "\n"

		switch item[3] {
		case "H264", "H265":
			p.Medias = append(p.Medias, &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{
						Name:        item[3],
						ClockRate:   90000,
						PayloadType: core.PayloadTypeRAW,
					},
				},
			})
			p.videoPT = byte(core.Atoi(item[2]))

		case "G711":
			p.Medias = append(p.Medias, &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{
						Name:      core.CodecPCMA,
						ClockRate: 8000,
					},
				},
			})
			p.audioPT = byte(core.Atoi(item[2]))
		}
	}

	return nil
}

func (p *Producer) Start() error {
	receivers := make(map[uint8]*core.Receiver)

	for _, receiver := range p.Receivers {
		switch receiver.Codec.Kind() {
		case core.KindVideo:
			receivers[p.videoPT] = receiver
		case core.KindAudio:
			receivers[p.audioPT] = receiver
		}
	}

	for {
		pkt, err := p.readPacket()
		if err != nil {
			return err
		}

		if recv := receivers[pkt.PayloadType]; recv != nil {
			switch recv.Codec.Name {
			case core.CodecH264, core.CodecH265:
				// timestamp = seconds x 1000000
				pkt = &rtp.Packet{
					Header: rtp.Header{
						Timestamp: uint32(uint64(pkt.Timestamp) * 90000 / 1000000),
					},
					Payload: annexb.EncodeToAVCC(pkt.Payload),
				}
			case core.CodecPCMA:
				pkt = &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						SequenceNumber: pkt.SequenceNumber,
						Timestamp:      uint32(uint64(pkt.Timestamp) * 8000 / 1000000),
					},
					Payload: pkt.Payload,
				}
			}
			recv.WriteRTP(pkt)
		}
	}
}

func (p *Producer) readPacket() (*core.Packet, error) {
	b := make([]byte, 8)

	if _, err := io.ReadFull(p.rd, b); err != nil {
		return nil, err
	}

	if b[0] != '$' {
		return nil, errors.New("eseecloud: wrong start byte")
	}

	size := binary.BigEndian.Uint32(b[4:])
	b = make([]byte, size)
	if _, err := io.ReadFull(p.rd, b); err != nil {
		return nil, err
	}

	pkt := &core.Packet{}
	if err := pkt.Unmarshal(b); err != nil {
		return nil, err
	}

	p.Recv += int(size)

	return pkt, nil
}
