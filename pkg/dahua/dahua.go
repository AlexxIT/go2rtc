package dahua

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Trace is an optional sink for raw protocol dumps (debug=1). The pkg layer
// owns no logger, so internal/dahua wires it to a zerolog "dahua" logger; while
// nil, traceHook returns nil and no frame is ever formatted.
var Trace func(format string, v ...any)

// Dial creates a Dahua two-way-audio (backchannel) consumer over the NetSDK
// binary talk path on TCP/37777 (CLIENT_StartTalkEx). CGI, speak.* JSON-RPC and
// DHIP are not implemented.
//
//	dahua://user:pass@host:37777?backchannel=0
//
// Params: backchannel (talk channel, 0-based), codec (pcma|pcmu|pcml|aac;
// pcma/pcmu are the only codecs a WebRTC browser can encode — pcml and aac
// negotiate only for non-browser sources), debug (1=dump frames), timeout (s,
// default 5). NetSDK-only: encodeformat (1=DH_TALK_PCM with-head PCM, the
// default; 0=DH_TALK_DEFAULT no-head PCM, 2=DH_TALK_G711a, etc. per the NetSDK
// enum. The demo UI only exposes 1, so other values are untested on-wire
// fallbacks), native (0=decode to PCM16, 1=forward G.711). See README for
// details.
func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	user := u.User.Username()
	pass, _ := u.User.Password()
	backchannel, _ := strconv.Atoi(query.Get("backchannel"))

	// Guard: Dahua talk channel must be >= 0. Single-IPC cameras (e.g. Dahua E4702)
	// only expose talk channel 0; a non-zero value is silently ignored by the device.
	if backchannel < 0 {
		return nil, fmt.Errorf("dahua: invalid backchannel=%d (must be >= 0)", backchannel)
	}

	codecName := forcedCodec(query.Get("codec"))
	timeout := dialTimeout(query.Get("timeout"))
	trace := traceHook(query.Get("debug") == "1")

	// The only backend is the NetSDK binary talk path on TCP/37777.
	transport := NewNetSDKTransport(u.Host, user, pass, backchannel, timeout)
	transport.forcedCodec = codecName
	if trace != nil {
		transport.Debug = trace
	}
	if v, err := strconv.Atoi(query.Get("encodeformat")); err == nil {
		transport.encodeFormat = v
	}
	// native=0 turns A-law passthrough off and decodes to PCM16 instead.
	// Only needed on firmware that refuses codec 0x0E upstream.
	if v := query.Get("native"); v != "" {
		transport.nativeG711 = v != "0" && v != "false"
	}

	return &Backchannel{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "dahua",
			Protocol:   "tcp",
			RemoteAddr: u.Host,
			URL:        rawURL,
			Medias:     backchannelMedias(transport.Codecs()),
		},
		transport: transport,
		stopped:   make(chan struct{}),
	}, nil
}

// forcedCodec maps the codec= query value onto a core codec name. An empty
// result falls back to the default codec list (PCMA, PCMU, PCML) returned by
// Codecs — go2rtc does not probe the device for its capabilities.
func forcedCodec(name string) string {
	switch name {
	case "pcma", "g711a", "G.711A":
		return core.CodecPCMA
	case "pcmu", "g711u", "G.711Mu":
		return core.CodecPCMU
	case "pcml", "pcm":
		return core.CodecPCML
	case "aac":
		// Reachable but last-resort: upstream notes "AAC has unknown problems
		// on Dahua two way" (internal/ffmpeg/producer.go).
		return core.CodecAAC
	}
	return ""
}

// dialTimeout parses the timeout= query value, in seconds.
func dialTimeout(v string) time.Duration {
	if n, _ := strconv.Atoi(v); n > 0 {
		return time.Duration(n) * time.Second
	}
	return 5 * time.Second
}

// traceHook builds the per-frame dump hook for debug=1. Returns nil when tracing
// is off or no sink is installed, keeping the expensive hex.Dump out of the
// hot path.
func traceHook(enabled bool) func(dir string, b []byte) {
	if !enabled || Trace == nil {
		return nil
	}

	return func(dir string, b []byte) {
		const maxDump = 160
		if len(b) > maxDump {
			Trace("%s (%d bytes, first %d)\n%s", dir, len(b), maxDump, hex.Dump(b[:maxDump]))
			return
		}
		Trace("%s (%d bytes)\n%s", dir, len(b), hex.Dump(b))
	}
}

// backchannelMedias is the single place we describe ourselves to the negotiator.
func backchannelMedias(codecs []*core.Codec) []*core.Media {
	return []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    negotiableCodecs(codecs),
		},
	}
}

// negotiableCodecs advertises what the transport can do, with two negotiation
// fixes (DHAV framing reads neither field):
//  1. PCMA wins alone when offered - matches upstream's "leave only one codec
//     here for better compatibility" rule, and A-law is the E4702's native
//     format (nativeG711 forwards it untouched). Forced codecs pass through.
//  2. Channels is cleared: core.Codec.Match only tolerates a zero on its right
//     operand, and the side flips between AddConsumer (browser) and Play
//     (ffmpeg). With Channels 1, Play answered "can't find consumer".
func negotiableCodecs(codecs []*core.Codec) []*core.Codec {
	for _, codec := range codecs {
		if codec.Name == core.CodecPCMA {
			return []*core.Codec{withoutChannels(codec)}
		}
	}

	out := make([]*core.Codec, 0, len(codecs))
	for _, codec := range codecs {
		out = append(out, withoutChannels(codec))
	}
	return out
}

// withoutChannels copies a codec with Channels cleared.
func withoutChannels(codec *core.Codec) *core.Codec {
	clone := *codec
	clone.Channels = 0
	return &clone
}
