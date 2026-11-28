# Built-in audio conversion

Consumers advertise additional input formats and convert them before their protocol's packet handler. Camera backchannels receive the same adapter when opened through a stream source handler. Protocol media descriptions and SDP keep the original codecs.

| Input | Output | Typical use |
| --- | --- | --- |
| PCM, PCML, PCMA, PCMU | Opus | WebRTC, HomeKit and clients requesting Opus |
| Opus | PCM, PCML, PCMA, PCMU | Camera speakers and audio playback |
| AAC-LC/AAC-ELD | PCM, PCML, PCMA, PCMU, Opus | Camera audio for WebRTC/HomeKit, speakers and audio playback |
| Opus or AAC-LC/AAC-ELD | PCM then FLAC | FLAC-only MSE/MP4/HLS outputs, for example MP4 `?audio=pcml` |

No extra stream source or FFmpeg process is needed for these routes. Streams try sources in their configured order. Native codecs precede conversion aliases within each consumer; an earlier source can be converted even if a later source offers a direct match. `ANY`/`ALL` requests retain passthrough behavior. `?mp4=flac` also accepts native AAC, so an AAC source remains passthrough with that filter; request only PCM/FLAC to decode AAC.

## Supported outputs

WebRTC, HomeKit, RTSP, WebCodecs and MP4 opt in at their audio boundary. RTSP, Tapo, DVRIP, ISAPI, DoorBird, Wyze, Xiaomi MISS, Tuya and MultiTrans camera sources use the common backchannel adapter for PCM/G.711/Opus outputs. Wyoming, ALSA and exec PCM outputs are also supported. Wyoming microphone consumers opt in directly.

Mono and stereo are supported. Concrete PCM outputs with omitted channels are mono. Stereo conversion preserves independent channels during resampling. The existing FLAC writer is mono, so stereo is downmixed without changing the sample rate.

## Codec limits

- [go-opus v0.1.1](https://github.com/skrashevich/go-opus/releases/tag/v0.1.1) encodes 20 ms CELT packets at 48 kHz. The decoder accepts SILK, CELT and hybrid packets, rejects duplicate/late packets and attempts PLC for up to three lost packets. Unsupported PLC durations preserve the source timeline. FEC is not requested by this adapter. HomeKit's 16 kHz talker clock is normalized to the 48 kHz Opus clock.
- [go-aac](https://github.com/skrashevich/go-aac) decodes AAC-LC with 1024 samples and AAC-ELD with 512/480 samples per access unit, in mono or stereo. AAC-ELD SBR and resilience tools are unsupported. The adapter accepts raw/ADTS units and complete, non-interleaved MPEG4-GENERIC AAC-hbr RTP packets with 13-bit size and zero 3-bit index/delta fields. AudioSpecificConfig must be present and agree with the advertised sample rate and channel count. HomeKit ELD uses its advertised playback clock for PCM output while retaining the source's existing decoder config for passthrough compatibility. Fragmented RTP access units and unsupported header layouts are rejected.
- AAC encoding, ELD SBR, HE-AAC, MP3, multichannel audio and video conversion still require other processing such as FFmpeg. AAC-only camera speakers need AAC encoding and therefore still need FFmpeg.

## Implementation

`WithTranscoding` adds local aliases, `AddTrack` resolves them to a native output, and packet handlers perform the conversion. `WithProducer` decorates source backchannels once and forwards connection information, JSON and lifecycle methods. Standalone users of protocol packages can call it explicitly. The adapter keeps no connection-level codec registry or alias cache. Stream matching, playback matching and `core.Node` are unchanged.

Each handler owns its codec state. go-opus automatically reclaims native resources when its handlers become unreachable; go2rtc needs no manual TLS, unsafe buffer management or codec-specific shutdown hooks. go-aac owns ordinary Go state. Both work with `CGO_ENABLED=0`.
