# Notes

go2rtc tries to name formats, protocols and codecs the same way they are named in FFmpeg.
Some formats and protocols go2rtc supports exclusively. They have no equivalent in FFmpeg.

## Producers (input)

- The initiator of the connection can be go2rtc - **Source protocols**
- The initiator of the connection can be an external program - **Ingress protocols**
- Codecs can be incoming - **Receiver codecs**
- Codecs can be outgoing (two way audio) - **Sender codecs**

| Group      | Format       | Protocols       | Ingress | Receiver codecs                 | Sender codecs      | Example       |
|------------|--------------|-----------------|---------|---------------------------------|---------------------|---------------|
| Devices    | alsa         | pipe            |         |                                 | pcm                 | `alsa:`       |
| Devices    | v4l2         | pipe            |         |                                 |                     | `v4l2:`       |
| Files      | adts         | http, tcp, pipe | http    | aac                             |                     | `http:`       |
| Files      | flv          | http, tcp, pipe | http    | h264, aac                       |                     | `http:`       |
| Files      | h264         | http, tcp, pipe | http    | h264                            |                     | `http:`       |
| Files      | hevc         | http, tcp, pipe | http    | hevc                            |                     | `http:`       |
| Files      | hls          | http            |         | h264, h265, aac, opus           |                     | `http:`       |
| Files      | mjpeg        | http, tcp, pipe | http    | mjpeg                           |                     | `http:`       |
| Files      | mpegts       | http, tcp, pipe | http    | h264, hevc, aac, opus           |                     | `http:`       |
| Files      | wav          | http, tcp, pipe | http    | pcm_alaw, pcm_mulaw             |                     | `http:`       |
| Net (pub)  | mpjpeg       | http, tcp, pipe | http    | mjpeg                           |                     | `http:`       |
| Net (pub)  | onvif        | rtsp            |         |                                 |                     | `onvif:`      |
| Net (pub)  | rtmp         | rtmp            | rtmp    | h264, aac                       |                     | `rtmp:`       |
| Net (pub)  | rtsp         | rtsp, ws        | rtsp    | h264, hevc, aac, pcm*, opus     | pcm*, opus          | `rtsp:`       |
| Net (pub)  | webrtc*      | webrtc          | webrtc  | h264, pcm_alaw, pcm_mulaw, opus | pcm_alaw, pcm_mulaw | `webrtc:`     |
| Net (pub)  | yuv4mpegpipe | http, tcp, pipe | http    | rawvideo                        |                     | `http:`       |
| Net (priv) | bubble       | http            |         | h264, hevc, pcm_alaw            |                     | `bubble:`     |
| Net (priv) | doorbird     | http            |         |                                 |                     | `doorbird:`   |
| Net (priv) | dvrip        | tcp             |         | h264, hevc, pcm_alaw, pcm_mulaw | pcm_alaw            | `dvrip:`      |
| Net (priv) | eseecloud    | http            |         |                                 |                     | `eseecloud:`  |
| Net (priv) | gopro        | udp             |         | TODO                            |                     | `gopro:`      |
| Net (priv) | hass         | webrtc          |         | TODO                            |                     | `hass:`       |
| Net (priv) | homekit      | hap             |         | h264, eld*                      |                     | `homekit:`    |
| Net (priv) | isapi        | http            |         |                                 | pcm_alaw, pcm_mulaw | `isapi:`      |
| Net (priv) | kasa         | http            |         | h264, pcm_mulaw                 |                     | `kasa:`       |
| Net (priv) | nest         | rtsp, webrtc    |         | TODO                            |                     | `nest:`       |
| Net (priv) | ring         | webrtc          |         |                                 |                     | `ring:`       |
| Net (priv) | roborock     | webrtc          |         | h264, opus                      | opus                | `roborock:`   |
| Net (priv) | tapo         | http            |         | h264, pcma                      | pcm_alaw            | `tapo:`       |
| Net (priv) | tuya         | webrtc          |         |                                 |                     | `tuya:`       |
| Net (priv) | vigi         | http            |         |                                 |                     | `vigi:`       |
| Net (priv) | webtorrent   | webrtc          | TODO    | TODO                            | TODO                | `webtorrent:` |
| Net (priv) | xiaomi*      | cs2, tutk       |         |                                 |                     | `xiaomi:`     |
| Services   | flussonic    | ws              |         |                                 |                     | `flussonic:`  |
| Services   | ivideon      | ws              |         | h264                            |                     | `ivideon:`    |
| Services   | yandex       | webrtc          |         |                                 |                     | `yandex:`     |
| Other      | echo         | *               |         |                                 |                     | `echo:`       |
| Other      | exec         | pipe, rtsp      |         |                                 |                     | `exec:`       |
| Other      | expr         | *               |         |                                 |                     | `expr:`       |
| Other      | ffmpeg       | pipe, rtsp      |         |                                 |                     | `ffmpeg:`     |
| Other      | stdin        | pipe            |         |                                 | pcm_alaw, pcm_mulaw | `stdin:`      |

- **eld** - rare variant of aac codec
- **pcm** - pcm_alaw pcm_mulaw pcm_s16be pcm_s16le
- **webrtc** - webrtc/kinesis, webrtc/openipc, webrtc/milestone, webrtc/wyze, webrtc/whep

## Consumers (output)

| Format       | Protocol | Send codecs                     | Recv codecs               | Example                               |
|--------------|----------|---------------------------------|---------------------------|---------------------------------------|
| adts         | http     | aac                             |                           | `GET /api/stream.adts`                |
| ascii        | http     | mjpeg                           |                           | `GET /api/stream.ascii`               |
| flv          | http     | h264, aac                       |                           | `GET /api/stream.flv`                 |
| hls/mpegts   | http     | h264, hevc, aac                 |                           | `GET /api/stream.m3u8`                |
| hls/fmp4     | http     | h264, hevc, aac, pcm*, opus     |                           | `GET /api/stream.m3u8?mp4`            |
| homekit      | hap      | h264, opus                      |                           | Apple HomeKit app                     |
| mjpeg        | ws       | mjpeg                           |                           | `{"type":"mjpeg"}` -> `/api/ws`       |
| mpjpeg       | http     | mjpeg                           |                           | `GET /api/stream.mjpeg`               |
| mp4          | http     | h264, hevc, aac, pcm*, opus     |                           | `GET /api/stream.mp4`                 |
| mse/fmp4     | ws       | h264, hevc, aac, pcm*, opus     |                           | `{"type":"mse"}` -> `/api/ws`         |
| mpegts       | http     | h264, hevc, aac                 |                           | `GET /api/stream.ts`                  |
| rtmp         | rtmp     | h264, aac                       |                           | `rtmp://localhost:1935/{stream_name}` |
| rtsp         | rtsp     | h264, hevc, aac, pcm*, opus     |                           | `rtsp://localhost:8554/{stream_name}` |
| webrtc       | webrtc   | h264, pcm_alaw, pcm_mulaw, opus | pcm_alaw, pcm_mulaw, opus | `{"type":"webrtc"}` -> `/api/ws`      |
| yuv4mpegpipe | http     | rawvideo                        |                           | `GET /api/stream.y4m`                 |

- **pcm** - pcm_alaw pcm_mulaw pcm_s16be pcm_s16le

## Snapshots

| Format | Protocol | Send codecs | Example               |
|--------|----------|-------------|-----------------------|
| jpeg   | http     | mjpeg       | `GET /api/frame.jpeg` |
| mp4    | http     | h264,hevc   | `GET /api/frame.mp4`  |

## Developers

**File naming:**

- `pkg/{format}/producer.go` - producer for this format (also if support backchannel)
- `pkg/{format}/consumer.go` - consumer for this format
- `pkg/{format}/backchannel.go` - producer with only backchannel func

**Mentioning modules:**

- [`main.go`](../main.go)
- [`README.md`](../README.md)
- [`internal/README.md`](../internal/README.md)
- [`website/.vitepress/config.js`](../website/.vitepress/config.js)
- [`website/api/openapi.yaml`](../website/api/openapi.yaml)
- [`www/schema.json`](../www/schema.json)

## Builds

The project has no `CGO` dependencies, so building is as simple as possible using the `go build` command.

The project has to use the latest versions of go due to dependencies on third-party go libraries. Such as `pion/webrtc` or `golang.org/x`. Unfortunately, this breaks compatibility with older versions of operating systems.

The project uses [UPX](https://github.com/upx/upx) to compress binaries for Linux. The project does not use compression for Windows due to false antivirus alarms. The project does not use compression for macOS due to broken result.

## Virus

- https://go.dev/doc/faq#virus
- https://groups.google.com/g/golang-nuts/c/lPwiWYaApSU

## Dependencies

```
- github.com/AlexxIT/go2rtc/pkg/yaml
  - gopkg.in/yaml.v3
- github.com/AlexxIT/go2rtc/pkg/hap
  - github.com/tadglines/go-pkgs
  - golang.org/x/crypto
- github.com/AlexxIT/go2rtc/pkg/mdns
  - github.com/miekg/dns
    - golang.org/x/mod
    - golang.org/x/net
    - golang.org/x/sync
    - golang.org/x/sys
    - golang.org/x/tools
- github.com/AlexxIT/go2rtc/pkg/pcm
  - github.com/pion/rtp
  - github.com/sigurn/crc16
  - github.com/sigurn/crc8
- github.com/AlexxIT/go2rtc/pkg/expr
  - github.com/expr-lang/expr
- github.com/AlexxIT/go2rtc/pkg/rtsp
  - github.com/pion/rtcp
  - github.com/pion/rtp
  - github.com/pion/sdp/v3
- github.com/AlexxIT/go2rtc/pkg/srtp
  - github.com/pion/rtcp
  - github.com/pion/rtp
  - github.com/pion/srtp/v3
- github.com/AlexxIT/go2rtc/pkg/webrtc
  - github.com/pion/ice/v4
    - github.com/google/uuid
    - github.com/pion/dtls/v3
    - github.com/pion/logging
    - github.com/pion/mdns/v2
    - github.com/pion/randutil
    - github.com/pion/stun/v4
    - github.com/pion/transport/v5
    - github.com/pion/turn/v5
    - github.com/wlynxg/anet
    - go.yaml.in/yaml/v3
    - golang.org/x/crypto
    - golang.org/x/net
    - golang.org/x/sys
    - golang.org/x/time
  - github.com/pion/interceptor
  - github.com/pion/rtcp
  - github.com/pion/rtp
  - github.com/pion/sdp/v3
  - github.com/pion/stun/v3
    - github.com/pion/transport/v4
  - github.com/pion/webrtc/v4
    - github.com/pion/datachannel
    - github.com/pion/dtls/v3
    - github.com/pion/ice/v4
    - github.com/pion/interceptor
    - github.com/pion/logging
    - github.com/pion/mdns/v2
    - github.com/pion/randutil
    - github.com/pion/rtcp
    - github.com/pion/rtp
    - github.com/pion/sctp
    - github.com/pion/sdp/v3
    - github.com/pion/srtp/v3
    - github.com/pion/stun/v4
    - github.com/pion/transport/v5
    - github.com/pion/turn/v5
    - github.com/wlynxg/anet
    - go.yaml.in/yaml/v3
    - golang.org/x/crypto
    - golang.org/x/net
    - golang.org/x/sys
    - golang.org/x/time
- github.com/AlexxIT/go2rtc/pkg/tutk/dtls
  - github.com/pion/dtls/v3
  - golang.org/x/crypto
- github.com/AlexxIT/go2rtc/pkg/tuya
  - github.com/eclipse/paho.mqtt.golang
    - github.com/gorilla/websocket
    - golang.org/x/net
    - golang.org/x/sync
  - github.com/google/uuid
  - github.com/pion/rtp
  - github.com/pion/webrtc/v4
  - golang.org/x/net
- github.com/AlexxIT/go2rtc/pkg/multitrans
  - github.com/google/uuid
  - github.com/pion/rtp
- github.com/AlexxIT/go2rtc/pkg/ring
  - github.com/google/uuid
  - github.com/gorilla/websocket
  - github.com/pion/rtp
  - github.com/pion/webrtc/v4
- github.com/AlexxIT/go2rtc/pkg/flussonic
  - github.com/gorilla/websocket
- github.com/AlexxIT/go2rtc/pkg/ivideon
  - github.com/gorilla/websocket
- github.com/rs/zerolog
  - github.com/mattn/go-colorable
    - github.com/mattn/go-isatty
    - golang.org/x/sys
  - github.com/mattn/go-isatty
    - golang.org/x/sys
```

## Licenses

- github.com/eclipse/paho.mqtt.golang - EPL-2.0 or BSD-3-Clause
- github.com/expr-lang/expr - MIT
- github.com/google/uuid - BSD-3-Clause
- github.com/gorilla/websocket - BSD-2-Clause
- github.com/mattn/go-colorable - MIT
- github.com/mattn/go-isatty - MIT
- github.com/miekg/dns - BSD-3-Clause
- github.com/pion/datachannel - MIT
- github.com/pion/dtls/v3 - MIT
- github.com/pion/ice/v4 - MIT
- github.com/pion/interceptor - MIT
- github.com/pion/logging - MIT
- github.com/pion/mdns/v2 - MIT
- github.com/pion/randutil - MIT
- github.com/pion/rtcp - MIT
- github.com/pion/rtp - MIT
- github.com/pion/sctp - MIT
- github.com/pion/sdp/v3 - MIT
- github.com/pion/srtp/v3 - MIT
- github.com/pion/stun/v3 - MIT
- github.com/pion/stun/v4 - MIT
- github.com/pion/transport/v4 - MIT
- github.com/pion/transport/v5 - MIT
- github.com/pion/turn/v5 - MIT
- github.com/pion/webrtc/v4 - MIT
- github.com/rs/zerolog - MIT
- github.com/sigurn/crc16 - MIT
- github.com/sigurn/crc8 - MIT
- github.com/stretchr/testify - MIT
- github.com/tadglines/go-pkgs - Apache-2.0
- github.com/wlynxg/anet - BSD-3-Clause
- go.yaml.in/yaml/v3 - MIT and Apache-2.0
- golang.org/x/crypto - BSD-3-Clause
- golang.org/x/net - BSD-3-Clause
- golang.org/x/sync - BSD-3-Clause
- golang.org/x/sys - BSD-3-Clause
- golang.org/x/time - BSD-3-Clause
- gopkg.in/yaml.v3 - MIT and Apache-2.0

## Useful links

**Development**

- https://github.com/golang-standards/project-layout
- https://github.com/micro/micro
- https://github.com/golang/go/wiki/GoArm
- https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63
- https://en.wikipedia.org/wiki/AArch64
- https://stackoverflow.com/questions/22267189/what-does-the-w-flag-mean-when-passed-in-via-the-ldflags-option-to-the-go-comman

**Media**

- https://www.wowza.com/blog/streaming-protocols
- https://vimeo.com/blog/post/rtmp-stream/
- https://sanjeev-pandey.medium.com/understanding-the-mpeg-4-moov-atom-pseudo-streaming-in-mp4-93935e1b9e9a
- [Android Supported media formats](https://developer.android.com/guide/topics/media/media-formats)
- [THEOplayer](https://www.theoplayer.com/test-your-stream-hls-dash-hesp)
- [How Generate DTS/PTS](https://www.ramugedia.com/how-generate-dts-pts-from-elementary-stream)
