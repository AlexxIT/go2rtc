# go2rtc

**go2rtc** - ultimate camera streaming application with support RTSP, WebRTC, FFmpeg, RTMP, etc.

- zero-dependency and zero-config small [app for all OS](#installation) (Windows, macOS, Linux, ARM)
- zero-delay for all supported protocols (lowest possible streaming latency)
- zero-load on CPU for supported codecs
- on the fly transcoding for unsupported codecs [via FFmpeg](#source-ffmpeg)
- multi-source 2-way [codecs negotiation](#codecs-negotiation)
- streaming from private networks via [Ngrok or SSH-tunnels](#module-webrtc)

## Codecs negotiation

For example, you want to watch RTSP-stream from [Dahua IPC-K42](https://www.dahuasecurity.com/fr/products/All-Products/Network-Cameras/Wireless-Series/Wi-Fi-Series/4MP/IPC-K42) camera in your Chrome browser.

- this camera support 2-way audio standard **ONVIF Profile T**
- this camera support codecs **H264, H265** for send video, and you select `H264` in camera settings
- this camera support codecs **AAC, PCMU, PCMA** for send audio (from mic), and you select `AAC/16000` in camera settings
- this camera support codecs **AAC, PCMU, PCMA** for receive audio (to speaker), you don't need to select them
- your browser support codecs **H264, VP8, VP9, AV1** for receive video, you don't need to select them
- your browser support codecs **OPUS, PCMU, PCMA** for send and receive audio, you don't need to select them
- you can't get camera audio directly, because its audio codecs doesn't match with your browser codecs 
   - so you decide to use transcoding via FFmpeg and add this setting to config YAML file
   - you have chosen `OPUS/48000/2` codec, because it is higher quality than the `PCMU/8000` or `PCMA/8000`
- now you have stream with two sources - **RTSP and FFmpeg**

**go2rtc** automatically match codecs for you browser and all your stream sources. This called **multi-source 2-way codecs negotiation**. And this is one of the main features of this app.

**PS.** You can select `PCMU` or `PCMA` codec in camera setting and don't use transcoding at all. Or you can select `AAC` codec for main stream and `PCMU` codec for second stream and add both RTSP to YAML config, this also will work fine.

```yaml
streams:
  dahua:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - ffmpeg:rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif#audio=opus
```

![](codecs.svg)

## Installation

Download binary for your OS from [latest release](https://github.com/AlexxIT/go2rtc/releases/):

- `go2rtc_win64.exe` - Windows 64-bit
- `go2rtc_win32.exe` - Windows 32-bit
- `go2rtc_linux_amd64` - Linux 64-bit
- `go2rtc_linux_i386` - Linux 32-bit
- `go2rtc_linux_arm64` - Linux ARM 64-bit (ex. Raspberry 64-bit OS)
- `go2rtc_linux_arm` - Linux ARM 32-bit (ex. Raspberry 32-bit OS)
- `go2rtc_linux_mipsel` - Linux on MIPS (ex. [Xiaomi Gateway 3](https://github.com/AlexxIT/XiaomiGateway3))
- `go2rtc_mac_amd64` - Mac with Intel
- `go2rtc_mac_arm64` - Mac with M1

Don't forget to fix the rights `chmod +x go2rtc_linux_xxx` on Linux and Mac.

## Configuration

Create file `go2rtc.yaml` next to the app.

- by default, you need to config only your `streams` links
- `api` server will start on default **3000 port**
- `rtsp` server will start on default **554 port**
- `webrtc` will use random UDP port for each connection
- `ffmpeg` will use default transcoding options (you need to install it [manually](https://ffmpeg.org/))

Available modules:

- [streams](#module-streams)
- [api](#module-api) - HTTP API (important for WebRTC support)
- [rtsp](#module-rtsp) - RTSP Server (important for FFmpeg support)
- [webrtc](#module-webrtc) - WebRTC Server (important for external access)
- [ngrok](#module-ngrok) - Ngrok integration (external access for private network)
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [hass](#source-hass) - Home Assistant integration
- [log](#module-log) - logs config

### Module: Streams

**go2rtc** support different stream source types. You can config only one link as stream source or multiple.

Available source types:

- [rtsp](#source-rtsp) - most cameras on market
- [rtmp](#source-rtmp)
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [exec](#source-exec) - advanced FFmpeg and GStreamer integration
- [hass](#source-hass) - Home Assistant integration

#### Source: RTSP

- Support **RTSP and RTSPS** links with multiple video and audio tracks
- Support **2-way audio** ONLY for [ONVIF Profile T](https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf) cameras (back channel connection)

**Attention:** proprietary 2-way audio standards are not supported!

```yaml
streams:
  sonoff_camera: rtsp://rtsp:12345678@192.168.1.123:554/av_stream/ch0
```

If your camera has two RTSP links - you can add both of them as sources. This is useful when streams has different codecs, as example AAC audio with main stream and PCMU/PCMA audio with second stream.

**Attention:** Dahua cameras has different capabilities for different RTSP links. For example, it has support multiple codecs for 2-way audio with `&proto=Onvif` in link and only one codec without it.

```yaml
streams:
  dahua_camera:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=1
```

#### Source: RTMP

You can get stream from RTMP server, for example [Frigate](https://docs.frigate.video/configuration/rtmp). Support ONLY `H264` video codec without audio.

```yaml
streams:
  rtmp_stream: rtmp://192.168.1.123/live/camera1
```

#### Source: FFmpeg

You can get any stream or file or device via FFmpeg and push it to go2rtc. The app will automatically start FFmpeg with the proper arguments when someone starts watching the stream.

Format: `ffmpeg:{input}#{params}`. Examples:

```yaml
streams:
  # [FILE] all tracks will be copied without transcoding codecs
  file1: ffmpeg:~/media/BigBuckBunny.mp4

  # [FILE] video will be transcoded to H264, audio will be skipped
  file2: ffmpeg:~/media/BigBuckBunny.mp4#video=h264

  # [FILE] video will be copied, audio will be transcoded to pcmu
  file3: ffmpeg:~/media/BigBuckBunny.mp4#video=copy&audio=pcmu

  # [HLS] video will be copied, audio will be skipped
  hls: ffmpeg:https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear5/prog_index.m3u8#video=copy

  # [MJPEG] video will be transcoded to H264
  mjpeg: ffmpeg:http://185.97.122.128/cgi-bin/faststream.jpg?stream=half&fps=15#video=h264

  # [RTSP] video and audio will be copied
  rtsp: ffmpeg:rtsp://rtsp:12345678@192.168.1.123:554/av_stream/ch0#video=copy&audio=copy
```

All trascoding formats has built-in templates. But you can override them via YAML config. You can also add your own formats to config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg                                        # path to ffmpeg binary
  link: -hide_banner -i {input}                      # if input is link
  file: -hide_banner -re -stream_loop -1 -i {input}  # if input not link
  rtsp: -hide_banner -fflags nobuffer -flags low_delay -rtsp_transport tcp -i {input}  # if input is RTSP link
  output: -rtsp_transport tcp -f rtsp {output}  # output

  h264:       "-codec:v libx264 -g 30 -preset superfast -tune zerolatency -profile main -level 4.1"
  h264/ultra: "-codec:v libx264 -g 30 -preset ultrafast -tune zerolatency"
  h264/high:  "-codec:v libx264 -g 30 -preset superfast -tune zerolatency"
  h265:       "-codec:v libx265 -g 30 -preset ultrafast -tune zerolatency"
  opus:       "-codec:a libopus -ar 48000 -ac 2"
  pcmu:       "-codec:a pcm_mulaw -ar 8000 -ac 1"
  pcmu/16000: "-codec:a pcm_mulaw -ar 16000 -ac 1"
  pcmu/48000: "-codec:a pcm_mulaw -ar 48000 -ac 1"
  pcma:       "-codec:a pcm_alaw -ar 8000 -ac 1"
  pcma/16000: "-codec:a pcm_alaw -ar 16000 -ac 1"
  pcma/48000: "-codec:a pcm_alaw -ar 48000 -ac 1"
  aac/16000:  "-codec:a aac -ar 16000 -ac 1"
```

#### Source: Exec

FFmpeg source just a shortcut to exec source. You can get any stream or file or device via FFmpeg or GStreamer and push it to go2rtc via RTSP protocol: 

```yaml
streams:
  stream1: exec:ffmpeg -hide_banner -re -stream_loop -1 -i ~/media/BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp {output}
```

#### Source: Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- support ONLY [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI

```yaml
hass:
  config: "~/.homeassistant"

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
```

### Module: API

The HTTP API is the main part for interacting with the application.

- you can use WebRTC only when HTTP API enabled
- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "localhost:3000"` setting
- you can change API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`

```yaml
api:
  listen: ":3000"    # HTTP API port ("" - disabled)
  base_path: ""      # API prefix for serve on suburl
  static_dir: "www"  # folder for static files ("" - disabled)
```

### Module: RTSP

You can get any stream as RTSP-stream with codecs filter:

```
rtsp://192.168.1.123/{stream_name}?video={codec}&audio={codec1}&audio={codec2}
```

- you can omit the codecs, so one first video and one first audio will be selected
- you can set `?video=copy` or just `?video`, so only one first video without audio will be selected
- you can set multiple video or audio, so all of them will be selected

```yaml
rtsp:
  listen: ":554"
```

### Module: WebRTC

TODO...

```yaml
webrtc:
  listen: ":8555"  # address of your local server (TCP)
  candidates:
    - 216.58.210.174:8555  # if you have static public IP-address
    - 192.168.1.123:8555   # ip you have problems with UDP in LAN
    - stun   # if you have dynamic public IP-address (auto discovery via STUN)
  ice_servers:
    - urls: [stun:stun.l.google.com:19302]
    - urls: [turn:123.123.123.123:3478]
      username: your_user
      credential: your_pass
```

### Module: Ngrok

TODO...

```yaml
ngrok:
  command: ngrok tcp 8555 --authtoken eW91IHNoYWxsIG5vdCBwYXNzCnlvdSBzaGFsbCBub3QgcGFzcw
```

or

```yaml
ngrok:
  command: ngrok start --all --config ngrok.yml
```

### Module: Log

You can set different log levels for different modules.

```yaml
log:
  level: info  # default level
  api: trace
  exec: debug
  ngrok: info
  rtsp: warn
  streams: error
  webrtc: fatal
```
