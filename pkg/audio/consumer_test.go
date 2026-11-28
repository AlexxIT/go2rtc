package audio

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/pion/rtp"
	goopus "github.com/skrashevich/go-opus"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestLocalCapabilities(t *testing.T) {
	for _, name := range []string{core.CodecOpus, core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML} {
		t.Run(name, func(t *testing.T) {
			codec := &core.Codec{Name: name, ClockRate: 48000, Channels: 2}
			media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}}
			recv := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{codec}}
			local := WithTranscoding([]*core.Media{media, recv})
			if len(media.Codecs) != 1 || local[1] != recv || local[0].Codecs[0] != codec {
				t.Fatal("changed protocol media or direct codec")
			}
			input, rate := core.CodecOpus, uint32(48000)
			if name == core.CodecOpus {
				input, rate = core.CodecPCMA, 8000
			}
			for channels := range uint8(3) {
				source := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: input, ClockRate: rate, Channels: channels}}}
				for _, reverse := range []bool{false, true} {
					var alias *core.Codec
					if reverse {
						alias, _ = local[0].MatchMedia(source)
					} else {
						_, alias = source.MatchMedia(local[0])
					}
					if alias == nil {
						t.Fatalf("no alias for channels=%d reverse=%v", channels, reverse)
					}
				}
			}
		})
	}
}

func TestConversionRejectsMultichannel(t *testing.T) {
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}}}
	local := WithTranscoding([]*core.Media{media})[0]
	input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecPCML, ClockRate: 48000, Channels: 3}}}
	_, alias := input.MatchMedia(local)
	if alias == nil {
		t.Fatal("missing wildcard alias")
	}
	err := AddTrack([]*core.Media{media}, local, alias, core.NewReceiver(input, input.Codecs[0]), func(*core.Media, *core.Codec, *core.Receiver) error {
		t.Fatal("forwarded unconverted multichannel PCM to Opus")
		return nil
	})
	if err == nil {
		t.Fatal("accepted unsupported multichannel input")
	}
}

func TestDirectAndWildcardPassthrough(t *testing.T) {
	for _, name := range []string{core.CodecPCMA, core.CodecOpus, core.CodecAny, core.CodecAll} {
		codec := &core.Codec{Name: name}
		media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}}
		source := core.NewReceiver(nil, &core.Codec{Name: core.CodecPCMA, ClockRate: 8000})
		local := WithTranscoding([]*core.Media{media})[0]
		if err := AddTrack([]*core.Media{media}, local, codec, source, func(m *core.Media, c *core.Codec, r *core.Receiver) error {
			if m != media || c != codec || r != source {
				t.Fatal("changed a native track")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConversionAttachAndDetach(t *testing.T) {
	for _, dst := range []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}, {Name: core.CodecPCML, ClockRate: 22050}} {
		media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{dst}}
		local := WithTranscoding([]*core.Media{media})[0]
		src := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
		data := make([]byte, 160)
		if dst.Name != core.CodecOpus {
			src = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			data = encodeTestPacket(t, 2)
		}
		input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{src}}
		source := core.NewReceiver(input, src)
		_, alias := input.MatchMedia(local)
		rejected := errors.New("rejected")
		if err := AddTrack([]*core.Media{media}, local, alias, source, func(*core.Media, *core.Codec, *core.Receiver) error { return rejected }); !errors.Is(err, rejected) {
			t.Fatal(err)
		}
		if len(source.Senders()) != 0 {
			t.Fatal("failed AddTrack attached a filter")
		}
		var converted *core.Receiver
		var packet *core.Packet
		if err := AddTrack([]*core.Media{media}, local, alias, source, func(m *core.Media, c *core.Codec, r *core.Receiver) error {
			if m != media || c != dst || r.Codec.Name != dst.Name {
				t.Fatal("wrong protocol output")
			}
			converted = r
			r.AppendChild(&core.Node{Input: func(p *core.Packet) { packet = p }})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		source.WriteRTP(&core.Packet{Header: packetHeader(0), Payload: data})
		if packet == nil {
			t.Fatal("no converted output")
		}
		if dst.Name == core.CodecPCML && (len(packet.Payload) != 882 || converted.Codec.Channels != 1) {
			t.Fatal("concrete PCM output must be mono")
		}
		converted.Close()
		packet = nil
		source.WriteRTP(&core.Packet{Header: packetHeader(960), Payload: data})
		if packet != nil || len(source.Senders()) != 0 {
			t.Fatal("filter remains attached after track Close")
		}
	}
}

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

type camera struct {
	core.Connection
	starts, stops int
}

func (c *camera) Start() error { c.starts++; return nil }
func (c *camera) Stop() error  { c.stops++; return c.Connection.Stop() }
func (c *camera) AddTrack(m *core.Media, codec *core.Codec, r *core.Receiver) error {
	sender := core.NewSender(m, codec)
	sender.Handler = func(*core.Packet) {}
	sender.HandleRTP(r)
	c.Senders = append(c.Senders, sender)
	return nil
}
func TestProducerBoundary(t *testing.T) {
	for _, name := range []string{core.CodecPCML, core.CodecPCMA, core.CodecOpus} {
		t.Run(name, func(t *testing.T) {
			dst := &core.Codec{Name: name, ClockRate: 8000}
			if name == core.CodecOpus {
				dst.ClockRate = 48000
				dst.Channels = 2
			}
			native := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{dst}}
			camera := &camera{Connection: core.Connection{FormatName: "camera", Medias: []*core.Media{native}}}
			prod := WithProducer(camera)
			if prod == camera {
				t.Fatal("backchannel not adapted")
			}
			if WithProducer(prod) != prod {
				t.Fatal("wrapped producer twice")
			}
			prod.(core.Info).SetSource("camera:test")
			if camera.Source != "camera:test" {
				t.Fatal("Info not forwarded")
			}
			before, _ := json.Marshal(camera)
			after, _ := json.Marshal(prod)
			if !bytes.Equal(before, after) {
				t.Fatal("changed connection JSON")
			}
			medias := prod.GetMedias()
			if !slices.Equal(medias, WithTranscoding(medias)) {
				t.Fatal("non-idempotent aliases")
			}
			src := &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			if name == core.CodecOpus {
				src = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
			media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{src}}
			local := prod.GetMedias()[0]
			_, alias := media.MatchMedia(local)
			if alias == nil {
				t.Fatal("no conversion match")
			}
			receiver := core.NewReceiver(media, src)
			if err := prod.(core.Consumer).AddTrack(local, alias, receiver); err != nil {
				t.Fatal(err)
			}
			if camera.Senders[0].Codec != dst || len(native.Codecs) != 1 {
				t.Fatal("changed native media")
			}
			_ = prod.Start()
			_ = prod.Stop()
			if camera.starts != 1 || camera.stops != 1 || len(receiver.Senders()) != 0 {
				t.Fatal("lifecycle not forwarded")
			}
		})
	}
	for _, name := range []string{core.CodecAny, core.CodecAAC} {
		camera := &camera{Connection: core.Connection{Medias: []*core.Media{{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: name}}}}}}
		if WithProducer(camera) != camera {
			t.Fatal("wrapped unsupported or passthrough output")
		}
	}
}

// testdata/stereo.aac contains eight FFmpeg AAC-LC frames, including PNS.
// ffmpeg -f lavfi -i 'aevalsrc=0.2*sin(2*PI*1000*t)|0.2*sin(2*PI*1500*t):s=44100:d=1' -c:a aac -b:a 128k -f adts stereo.aac
func aacFrames(t *testing.T) [][]byte {
	t.Helper()
	data, err := os.ReadFile("testdata/stereo.aac")
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for len(data) >= 7 {
		size := int(aac.ReadADTSSize(data))
		if size < 7 || size > len(data) {
			t.Fatal("invalid AAC fixture")
		}
		frames = append(frames, data[:size])
		data = data[size:]
	}
	return frames
}
func TestAACInputs(t *testing.T) {
	frames := aacFrames(t)
	for _, format := range []string{"ADTS", "raw", "RTP"} {
		t.Run(format, func(t *testing.T) {
			src := aac.ADTSToCodec(frames[0])
			src.PayloadType = core.PayloadTypeRAW
			if format == "RTP" {
				src.PayloadType = 96
			}
			var output []*core.Packet
			h, err := TranscodeHandler(&core.Codec{Name: core.CodecPCML, ClockRate: 44100, Channels: 2}, src, func(p *core.Packet) { output = append(output, p) })
			if err != nil {
				t.Fatal(err)
			}
			for i, frame := range frames {
				payload := frame
				if format == "raw" {
					payload = frame[7:]
				}
				if format == "RTP" {
					payload = make([]byte, len(frame)-7+4)
					payload[1] = 16
					binary.BigEndian.PutUint16(payload[2:], uint16(len(frame)-7)<<3)
					copy(payload[4:], frame[7:])
				}
				h(&core.Packet{Header: packetHeader(10000 + uint32(i)*1024), Payload: payload})
			}
			if len(output) != len(frames) {
				t.Fatalf("decoded %d/%d AAC frames", len(output), len(frames))
			}
			var samples []int16
			for i, packet := range output {
				if packet.Timestamp != 10000+uint32(i)*1024 || len(packet.Payload) != 4096 {
					t.Fatalf("bad PCM packet %d: %+v", i, packet.Header)
				}
				for j := 0; j < len(packet.Payload); j += 2 {
					samples = append(samples, int16(binary.LittleEndian.Uint16(packet.Payload[j:])))
				}
			}
			for channel, expected := range []float64{1000, 1500} {
				var energy float64
				crossings, zeros, n := 0, 0, 0
				var previous int16
				for i := 4096 + channel; i < len(samples); i += 2 {
					v := samples[i]
					energy += float64(v) * float64(v)
					n++
					if v == 0 {
						zeros++
					}
					if previous <= 0 && v > 0 {
						crossings++
					}
					previous = v
				}
				hz := float64(crossings) * 44100 / float64(n)
				if math.Abs(hz-expected) > 40 || math.Sqrt(energy/float64(n)) < 1000 || zeros > n/20 {
					t.Fatalf("channel %d: Hz=%.1f RMS=%.1f zeros=%d/%d", channel, hz, math.Sqrt(energy/float64(n)), zeros, n)
				}
			}
		})
	}
}
func TestAACDestinations(t *testing.T) {
	frames := aacFrames(t)
	src := aac.ADTSToCodec(frames[0])
	src.PayloadType = core.PayloadTypeRAW
	for _, name := range []string{core.CodecOpus, core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML} {
		t.Run(name, func(t *testing.T) {
			dst := &core.Codec{Name: name, ClockRate: 8000, Channels: 1}
			if name == core.CodecOpus {
				dst.ClockRate = 48000
				dst.Channels = 2
			}
			var output []*core.Packet
			h, err := TranscodeHandler(dst, src, func(p *core.Packet) { output = append(output, p) })
			if err != nil {
				t.Fatal(err)
			}
			for i, frame := range frames {
				h(&core.Packet{Header: packetHeader(uint32(i) * 1024), Payload: frame})
			}
			if len(output) == 0 {
				t.Fatal("no AAC conversion output")
			}
			if name != core.CodecOpus {
				size := 0
				for _, p := range output {
					size += len(p.Payload)
				}
				unit := 1
				if name == core.CodecPCM || name == core.CodecPCML {
					unit = 2
				}
				expected := len(frames) * 1024 * 8000 / 44100 * unit
				if size != expected {
					t.Fatalf("resampling: got %d bytes, want %d", size, expected)
				}
			}
			if name == core.CodecOpus {
				d, err := goopus.NewDecoderWithOutput(48000, 2)
				if err != nil {
					t.Fatal(err)
				}
				for i, p := range output {
					n, err := d.DecodeToInt16(p.Payload, make([]int16, 1920))
					if err != nil || n != 960 || p.Timestamp != uint32(i*960) {
						t.Fatalf("invalid Opus: %d, %v", n, err)
					}
				}
			}
		})
	}
	for _, profile := range []byte{aac.TypeAACLD, 5} {
		invalid := aac.ConfigToCodec(aac.EncodeConfig(profile, 48000, 1, false))
		if _, err := TranscodeHandler(&core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}, invalid, func(*core.Packet) {}); err == nil {
			t.Fatalf("accepted AAC profile %d", profile)
		}
	}
}

func eldFrames(t *testing.T) (*core.Codec, [][]byte) {
	t.Helper()
	b, err := os.ReadFile("testdata/mono.eld")
	if err != nil {
		t.Fatal(err)
	}
	src := aac.ConfigToCodec(b[1 : 1+int(b[0])])
	b = b[1+int(b[0]):]
	var frames [][]byte
	for len(b) > 0 {
		n := int(binary.BigEndian.Uint16(b))
		frames = append(frames, b[2:2+n])
		b = b[2+n:]
	}
	return src, frames
}

func TestELDInputsAndFrameTiming(t *testing.T) {
	for _, format := range []string{"raw", "RTP", "HomeKit"} {
		t.Run(format, func(t *testing.T) {
			src, frames := eldFrames(t)
			if format != "raw" {
				src.PayloadType = 96
			}
			if format == "HomeKit" {
				src.Name, src.ClockRate = core.CodecELD, 16000
			}
			var packets []*core.Packet
			h, err := TranscodeHandler(&core.Codec{Name: core.CodecPCML, ClockRate: src.ClockRate, Channels: 1}, src, func(p *core.Packet) { packets = append(packets, p) })
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < len(frames); i += 2 {
				if format == "raw" {
					for j := range 2 {
						h(&core.Packet{Header: packetHeader(9000 + uint32(i+j)*480), Payload: frames[i+j]})
					}
					continue
				}
				payload := []byte{0, 32}
				for _, f := range frames[i : i+2] {
					payload = append(payload, byte(len(f)>>5), byte(len(f)<<3))
				}
				for _, f := range frames[i : i+2] {
					payload = append(payload, f...)
				}
				h(&core.Packet{Header: packetHeader(9000 + uint32(i)*480), Payload: payload})
			}
			if len(packets) != len(frames) {
				t.Fatalf("decoded %d/%d ELD frames", len(packets), len(frames))
			}
			for i, p := range packets {
				if p.Timestamp != 9000+uint32(i)*480 || len(p.Payload) != 960 {
					t.Fatalf("ELD frame %d: ts=%d bytes=%d", i, p.Timestamp, len(p.Payload))
				}
			}
		})
	}
}

func TestHomeKitELDDestinations(t *testing.T) {
	for _, name := range []string{core.CodecOpus, core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML} {
		t.Run(name, func(t *testing.T) {
			src, frames := eldFrames(t)
			src.Name, src.ClockRate = core.CodecELD, 16000
			input := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{src}}
			output := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: name, ClockRate: 8000, Channels: 1}}}
			if _, codec := input.MatchMedia(WithTranscoding([]*core.Media{output})[0]); codec == nil {
				t.Fatal("ELD source not advertised")
			}
			var packets []*core.Packet
			h, err := TranscodeHandler(output.Codecs[0], src, func(p *core.Packet) { packets = append(packets, p) })
			if err != nil {
				t.Fatal(err)
			}
			for i, f := range frames {
				h(&core.Packet{Header: packetHeader(uint32(i) * 480), Payload: f})
			}
			if name == core.CodecOpus {
				if len(packets) != 6 {
					t.Fatalf("120 ms ELD produced %d Opus packets, want 6", len(packets))
				}
				for i, p := range packets {
					if p.Timestamp != uint32(i)*960 {
						t.Fatalf("Opus timestamp %d", p.Timestamp)
					}
				}
			} else {
				unit := 1
				if name == core.CodecPCM || name == core.CodecPCML {
					unit = 2
				}
				if len(packets) != 4 {
					t.Fatalf("got %d packets", len(packets))
				}
				for i, p := range packets {
					if p.Timestamp != uint32(i)*240 || len(p.Payload) != 240*unit {
						t.Fatalf("PCM/G711 frame %d: ts=%d bytes=%d", i, p.Timestamp, len(p.Payload))
					}
				}
			}
		})
	}
}

func TestAACPacketValidation(t *testing.T) {
	frames := aacFrames(t)
	src := aac.ADTSToCodec(frames[0])
	src.PayloadType = 96
	var packets []*core.Packet
	h, err := aac.DecodeHandler(src, func(p *core.Packet) { packets = append(packets, p) })
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{nil, {0}, {0, 16}, {0, 16, 0, 8}, {0, 15, 0, 0}} {
		h(&core.Packet{Header: packetHeader(0), Payload: payload})
	}
	if len(packets) != 0 {
		t.Fatal("accepted malformed RTP")
	}
	payload := []byte{0, 32}
	for _, frame := range frames[:2] {
		n := len(frame) - 7
		payload = append(payload, byte(n>>5), byte(n<<3))
	}
	for _, frame := range frames[:2] {
		payload = append(payload, frame[7:]...)
	}
	h(&core.Packet{Header: packetHeader(9000), Payload: payload})
	if len(packets) != 2 || packets[0].Timestamp != 9000 || packets[1].Timestamp != 10024 {
		t.Fatal("incorrect bundled AU timestamps")
	}
	raw := src.Clone()
	raw.PayloadType = core.PayloadTypeRAW
	h, err = aac.DecodeHandler(raw, func(*core.Packet) { t.Fatal("accepted truncated AAC frame") })
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{nil, {0}, {0, 0, 0}} {
		h(&core.Packet{Payload: payload})
	}
	src.FmtpLine = strings.Replace(src.FmtpLine, "sizelength=13", "sizelength=6", 1)
	if _, err := aac.DecodeHandler(src, func(*core.Packet) {}); err == nil {
		t.Fatal("accepted unsupported AAC-hbr header size")
	}
}
