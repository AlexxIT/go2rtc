package opus

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	goopus "github.com/skrashevich/go-opus"
)

func packetHeader(ts uint32) rtp.Header { return rtp.Header{Version: 2, Timestamp: ts} }

func encodeTestPacket(t *testing.T, channels int) []byte {
	t.Helper()
	e, err := goopus.NewEncoder(goopus.WithChannels(channels))
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1276)
	n, err := e.Encode(make([]byte, 960*channels*2), data)
	if err != nil {
		t.Fatal(err)
	}
	return data[:n]
}

func TestEncodeHandlerFraming(t *testing.T) {
	for _, rate := range []uint32{8000, 16000, 22050, 48000} {
		t.Run((&core.Codec{Name: core.CodecPCML, ClockRate: rate}).String(), func(t *testing.T) {
			src := &core.Codec{Name: core.CodecPCML, ClockRate: rate, Channels: 1}
			dst := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: 111}
			var packets []*core.Packet
			h, err := TranscodeHandler(dst, src, func(p *core.Packet) { packets = append(packets, p) })
			if err != nil {
				t.Fatal(err)
			}
			// 40ms split into four 10ms input packets (including odd-rate input).
			previous := 0
			for i := range 4 {
				end := int(rate) * (i + 1) / 100
				h(&core.Packet{Header: packetHeader(uint32(previous)), Payload: make([]byte, 2*(end-previous))})
				previous = end
			}
			if len(packets) != 2 {
				t.Fatalf("got %d Opus packets", len(packets))
			}
			d, err := goopus.NewDecoderWithOutput(48000, 2)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range packets {
				if p.Timestamp != uint32(i*960) || p.PayloadType != 111 {
					t.Fatalf("RTP header: %+v", p.Header)
				}
				header := UnmarshalHeader(p.Payload)
				if header.FrameSize != 20*time.Millisecond || header.Frames != 1 {
					t.Fatalf("Opus framing: %+v", header)
				}
				n, err := d.DecodeToInt16(p.Payload, make([]int16, 1920))
				if err != nil || n != 960 {
					t.Fatalf("decode=%d, %v", n, err)
				}
			}
		})
	}
}

func TestEncodeHandlerTimestampGap(t *testing.T) {
	var packets []*core.Packet
	h, err := TranscodeHandler(&core.Codec{Name: core.CodecOpus}, &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}, func(p *core.Packet) { packets = append(packets, p) })
	if err != nil {
		t.Fatal(err)
	}
	h(&core.Packet{Header: packetHeader(0), Payload: make([]byte, 80)})
	h(&core.Packet{Header: packetHeader(240), Payload: make([]byte, 160)})
	if len(packets) != 1 || packets[0].Timestamp != 1440 {
		t.Fatalf("partial frame crossed a gap: %v", packets)
	}
}

func TestDecodeHandlerFormatsAndLoss(t *testing.T) {
	packet := encodeTestPacket(t, 2)
	for _, name := range []string{core.CodecPCML, core.CodecPCM, core.CodecPCMA, core.CodecPCMU} {
		t.Run(name, func(t *testing.T) {
			var packets []*core.Packet
			dst := &core.Codec{Name: name, ClockRate: 8000, Channels: 1}
			h, err := TranscodeHandler(dst, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}, func(p *core.Packet) { packets = append(packets, p) })
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range []uint16{65534, 65534, 0, 65535, 5} {
				ts := uint32(0)
				if i == 0 {
					ts = 1920
				} else if i == 5 {
					ts = 6720
				}
				hdr := packetHeader(ts)
				hdr.SequenceNumber = i
				h(&core.Packet{Header: hdr, Payload: packet})
			}
			want := []uint32{0, 160, 320, 1120}
			if len(packets) != len(want) {
				t.Fatalf("got %d packets", len(packets))
			}
			bytes := 160
			if isPCM(name) {
				bytes *= 2
			}
			for i, p := range packets {
				if p.Timestamp != want[i] || len(p.Payload) != bytes {
					t.Fatalf("packet %d: timestamp=%d payload=%d", i, p.Timestamp, len(p.Payload))
				}
			}
		})
	}
}

func TestEncodeHandlerPreservesStereo(t *testing.T) {
	pcm := make([]byte, 3840)
	for i := range 960 {
		left := int16(10000 * math.Sin(2*math.Pi*440*float64(i)/48000))
		right := int16(10000 * math.Sin(2*math.Pi*1200*float64(i)/48000))
		binary.LittleEndian.PutUint16(pcm[4*i:], uint16(left))
		binary.LittleEndian.PutUint16(pcm[4*i+2:], uint16(right))
	}
	var packet *core.Packet
	h, err := TranscodeHandler(&core.Codec{Name: core.CodecOpus, Channels: 2}, &core.Codec{Name: core.CodecPCML, ClockRate: 48000, Channels: 2}, func(p *core.Packet) { packet = p })
	if err != nil {
		t.Fatal(err)
	}
	h(&core.Packet{Payload: pcm})
	d, err := goopus.NewDecoderWithOutput(48000, 2)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, 1920)
	if _, err = d.DecodeToInt16(packet.Payload, out); err != nil {
		t.Fatal(err)
	}
	difference := int64(0)
	for i := range 960 {
		difference += int64(absSample(int(out[2*i]) - int(out[2*i+1])))
	}
	if difference < 100000 {
		t.Fatal("stereo channels collapsed to mono")
	}
}

func absSample(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestDecodeHandlerSILK60ms(t *testing.T) {
	e, err := goopus.NewEncoder()
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1276)
	n, err := e.EncodeSILK(make([]int16, 960), goopus.BandwidthWideband, data)
	if err != nil {
		t.Fatal(err)
	}
	var packets []*core.Packet
	h, err := TranscodeHandler(&core.Codec{Name: core.CodecPCML, ClockRate: 16000}, &core.Codec{Name: core.CodecOpus, ClockRate: 48000}, func(p *core.Packet) { packets = append(packets, p) })
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []uint16{0, 2} {
		hdr := packetHeader(uint32(i) * 2880)
		hdr.SequenceNumber = i
		h(&core.Packet{Header: hdr, Payload: data[:n]})
	}
	if len(packets) != 3 {
		t.Fatalf("got %d packets", len(packets))
	}
	for i, p := range packets {
		if p.Timestamp != uint32(i)*960 || len(p.Payload) != 1920 {
			t.Fatalf("60ms packet %d: ts=%d payload=%d", i, p.Timestamp, len(p.Payload))
		}
	}
}

func TestDecodeHandlerUnsupportedPLCDuration(t *testing.T) {
	var packets []*core.Packet
	h, err := TranscodeHandler(&core.Codec{Name: core.CodecPCML, ClockRate: 16000}, &core.Codec{Name: core.CodecOpus, ClockRate: 48000}, func(p *core.Packet) { packets = append(packets, p) })
	if err != nil {
		t.Fatal(err)
	}
	data := encodeTestPacket(t, 1)
	h(&core.Packet{Header: packetHeader(0), Payload: data})
	hdr := packetHeader(6720)
	hdr.SequenceNumber = 2
	h(&core.Packet{Header: hdr, Payload: data})
	if len(packets) != 2 || packets[1].Timestamp != 2240 {
		t.Fatal("unsupported PLC collapsed the audio timeline")
	}
}
