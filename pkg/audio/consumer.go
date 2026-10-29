package audio

import (
	"encoding/json"
	"slices"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
)

// WithTranscoding adds local input formats without modifying protocol medias.
// Like WebRTC's resampling aliases, these are only used to match input tracks.
func WithTranscoding(medias []*core.Media) []*core.Media {
	result := slices.Clone(medias)
	for i, media := range medias {
		if media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly {
			continue
		}
		target := conversionTarget(media.Codecs)
		if target == nil {
			continue
		}
		clone := *media
		clone.Codecs = slices.Clone(media.Codecs)
		for _, alias := range inputAliases {
			if alias.Name == core.CodecAAC || alias.Name == core.CodecELD || target.Name == core.CodecOpus || alias.Name == core.CodecOpus {
				appendCodec(&clone, alias)
			}
		}
		if len(clone.Codecs) != len(media.Codecs) {
			result[i] = &clone
		}
	}
	return result
}

func conversionTarget(codecs []*core.Codec) *core.Codec {
	if slices.ContainsFunc(codecs, func(codec *core.Codec) bool { return codec.Name == core.CodecAny || codec.Name == core.CodecAll }) {
		return nil
	}
	var target *core.Codec
	for _, codec := range codecs {
		if codec.Channels > 2 || slices.Contains(inputAliases, codec) {
			continue
		}
		if codec.Name == core.CodecOpus {
			return codec
		}
		if isLinearOrG711(codec.Name) && (target == nil || codec.Name == core.CodecPCML || isPCM(codec.Name) && !isPCM(target.Name)) {
			target = codec
		}
	}
	return target
}

func isPCM(name string) bool { return name == core.CodecPCM || name == core.CodecPCML }

// AddTrack resolves a conversion alias to the original protocol media/codec.
// Conversion state belongs to the packet handler; no connection-level registry
// or additional lifecycle hooks are needed.
func AddTrack(medias []*core.Media, media *core.Media, codec *core.Codec, track *core.Receiver, add func(*core.Media, *core.Codec, *core.Receiver) error) error {
	for _, original := range medias {
		if original == media {
			return add(media, codec, track)
		}
		if original.ID != media.ID || original.Kind != media.Kind || original.Direction != media.Direction || len(media.Codecs) < len(original.Codecs) || !slices.Equal(original.Codecs, media.Codecs[:len(original.Codecs)]) {
			continue
		}
		if slices.Contains(original.Codecs, codec) {
			return add(original, codec, track)
		}
		if !slices.Contains(media.Codecs[len(original.Codecs):], codec) {
			return add(original, codec, track)
		}
		target := conversionTarget(original.Codecs)
		if target == nil {
			return add(original, codec, track)
		}
		dst := target.Clone()
		if isLinearOrG711(dst.Name) && dst.ClockRate != 0 && dst.Channels == 0 {
			dst.Channels = 1
		}
		dst = outputCodec(dst, track.Codec)
		converted := core.NewReceiver(original, dst)
		handler, err := TranscodeHandler(dst, track.Codec, converted.Input)
		if err != nil {
			return err
		}
		converted.Input = handler
		if err = add(original, target, converted); err != nil {
			return err
		}
		converted.Node.WithParent(&track.Node)
		return nil
	}
	return add(media, codec, track)
}

func appendCodec(media *core.Media, codec *core.Codec) {
	if !slices.ContainsFunc(media.Codecs, func(c *core.Codec) bool { return *c == *codec }) {
		media.Codecs = append(media.Codecs, codec)
	}
}

func isLinearOrG711(name string) bool {
	return isPCM(name) || name == core.CodecPCMA || name == core.CodecPCMU
}

func outputCodec(dst, src *core.Codec) *core.Codec {
	dst = dst.Clone()
	if dst.Name == core.CodecOpus {
		dst.ClockRate = 48000
	}
	if dst.ClockRate == 0 {
		dst.ClockRate = src.ClockRate
		if dst.ClockRate == 0 {
			dst.ClockRate = 48000
		}
		if dst.Name == core.CodecPCMA || dst.Name == core.CodecPCMU {
			dst.ClockRate = 8000
		}
	}
	if dst.Channels == 0 {
		dst.Channels = max(src.Channels, 1)
		if dst.Name == core.CodecPCMA || dst.Name == core.CodecPCMU {
			dst.Channels = 1
		}
	}
	return dst
}

func TranscodeHandler(dst, src *core.Codec, handler core.HandlerFunc) (core.HandlerFunc, error) {
	if src == nil || dst == nil || src.Channels > 2 || dst.Channels > 2 {
		return nil, core.ErrCantGetTrack
	}
	dst = outputCodec(dst, src)
	if src.Name != core.CodecAAC && src.Name != core.CodecELD {
		return opus.TranscodeHandler(dst, src, handler)
	}
	input, err := aac.PCMCodec(src)
	if err != nil {
		return nil, err
	}
	if dst.Name == core.CodecOpus {
		handler, err = opus.TranscodeHandler(dst, input, handler)
		if err != nil {
			return nil, err
		}
	} else if isLinearOrG711(dst.Name) {
		convert := pcm.TranscodeChannels(dst, input)
		forward := handler
		handler = func(packet *core.Packet) {
			clone := *packet
			clone.Payload = convert(packet.Payload)
			clone.Timestamp = uint32(uint64(packet.Timestamp) * uint64(dst.ClockRate) / uint64(input.ClockRate))
			clone.PayloadType = dst.PayloadType
			forward(&clone)
		}
	} else {
		return nil, core.ErrCantGetTrack
	}
	return aac.DecodeHandler(src, handler)
}

// WithProducer adapts camera backchannels at the source boundary. Producers
// without a consumer or additional input formats retain their identity.
func WithProducer(prod core.Producer) core.Producer {
	if _, ok := prod.(interface{ audioAdapter() }); ok {
		return prod
	}
	cons, ok := prod.(core.Consumer)
	if !ok {
		return prod
	}
	medias := prod.GetMedias()
	if slices.Equal(medias, WithTranscoding(medias)) {
		return prod
	}
	wrapped := &backchannel{Producer: prod, consumer: cons}
	if info, ok := prod.(core.Info); ok {
		return &struct {
			*backchannel
			core.Info
		}{wrapped, info}
	}
	return wrapped
}

type backchannel struct {
	core.Producer
	consumer core.Consumer
}

func (c *backchannel) GetMedias() []*core.Media { return WithTranscoding(c.Producer.GetMedias()) }
func (c *backchannel) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	return AddTrack(c.Producer.GetMedias(), media, codec, track, c.consumer.AddTrack)
}
func (c *backchannel) MarshalJSON() ([]byte, error) { return json.Marshal(c.Producer) }

// Immutable input descriptions distinguish local aliases from native formats.
// Sharing these pointers also makes capability expansion idempotent.
var inputAliases = func() []*core.Codec {
	var aliases []*core.Codec
	for _, name := range []string{core.CodecOpus, core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML, core.CodecAAC, core.CodecELD} {
		aliases = append(aliases, &core.Codec{Name: name})
		rate := uint32(8000)
		if name == core.CodecOpus {
			rate = 48000
		} else if isPCM(name) || name == core.CodecAAC || name == core.CodecELD {
			continue
		}
		for channels := range uint8(3) {
			aliases = append(aliases, &core.Codec{Name: name, ClockRate: rate, Channels: channels})
		}
	}
	return aliases
}()

func (*backchannel) audioAdapter() {}
