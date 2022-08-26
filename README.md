# go2rtc

Ultimate camera streaming application with support RTSP, WebRTC, FFmpeg, RTMP, etc.

- zero-dependency and zero-config [small app](#go2rtc-binary) for all OS (Windows, macOS, Linux, ARM)
- zero-delay for all supported protocols (lowest possible streaming latency)
- low CPU load for supported codecs
- on the fly transcoding for unsupported codecs via [FFmpeg](#source-ffmpeg)
- multi-source 2-way [codecs negotiation](#codecs-negotiation)
- streaming from private networks via [Ngrok](#module-ngrok)
- can be [integrated to](#module-api) any smart home platform or be used as [standalone app](#go2rtc-binary)

**Inspired by:**

- series of streaming projects from [@deepch](https://github.com/deepch)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [rtsp-simple-server](https://github.com/aler9/rtsp-simple-server) idea from [@aler9](https://github.com/aler9)
- [GStreamer](https://gstreamer.freedesktop.org/) framework pipeline idea
- [MediaSoup](https://mediasoup.org/) framework routing idea

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

Now you have stream with two sources - **RTSP and FFmpeg**:

```yaml
streams:
  dahua:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - ffmpeg:rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0#audio=opus
```

**go2rtc** automatically match codecs for you browser and all your stream sources. This called **multi-source 2-way codecs negotiation**. And this is one of the main features of this app.

![](codecs.svg)

**PS.** You can select `PCMU` or `PCMA` codec in camera setting and don't use transcoding at all. Or you can select `AAC` codec for main stream and `PCMU` codec for second stream and add both RTSP to YAML config, this also will work fine.

## Fast start

1. Download [binary](#go2rtc-binary) or use [Docker](#go2rtc-docker) or [Home Assistant Add-on](#go2rtc-home-assistant-add-on)
2. Open web interface: `http://localhost:1984/`

**Optionally:**

- add your [streams](#module-streams) to [config](#configuration) file
- setup [external access](#module-webrtc) to webrtc
- setup [external access](#module-ngrok) to web interface
- install [ffmpeg](#source-ffmpeg) for transcoding

**Developers:**

- write your own [web interface](#module-api)
- integrate [web api](#module-api) into your smart home platform

### go2rtc: Binary

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

Don't forget to fix the rights `chmod +x go2rtc_xxx_xxx` on Linux and Mac.

### go2rtc: Home Assistant Add-on

[![](https://my.home-assistant.io/badges/supervisor_addon.svg)](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons)

1. Install Add-On:
    - Settings > Add-ons > Plus > Repositories > Add `https://github.com/AlexxIT/hassio-addons`
    - go2rtc > Install > Start
2. Setup [Integration](#module-hass)

**Optionally:**

- create `go2rtc.yaml` in your Home Assistant [config](https://www.home-assistant.io/docs/configuration) folder

### go2rtc: Docker

Container [alexxit/go2rtc](https://hub.docker.com/r/alexxit/go2rtc) with support `amd64`, `386`, `arm64`, `arm`. This container same as [Home Assistant Add-on](#go2rtc-home-assistant-add-on), but can be used separately from the Home Assistant. Container has preinstalled [FFmpeg](#source-ffmpeg) and [Ngrok](#module-ngrok) applications.

```yaml
services:
  go2rtc:
    image: alexxit/go2rtc
    network_mode: host
    restart: always
    volumes:
      - "~/go2rtc.yaml:/config/go2rtc.yaml"
```

## Configuration

Create file `go2rtc.yaml` next to the app.

- by default, you need to config only your `streams` links
- `api` server will start on default **1984 port**
- `rtsp` server will start on default **8554 port**
- `webrtc` will use random UDP port for each connection
- `ffmpeg` will use default transcoding options (you may install it [manually](https://ffmpeg.org/))

Available modules:

- [streams](#module-streams)
- [api](#module-api) - HTTP API (important for WebRTC support)
- [rtsp](#module-rtsp) - RTSP Server (important for FFmpeg support)
- [webrtc](#module-webrtc) - WebRTC Server
- [ngrok](#module-ngrok) - Ngrok integration (external access for private network)
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [hass](#module-hass) - Home Assistant integration
- [log](#module-log) - logs config

### Module: Streams

**go2rtc** support different stream source types. You can config one or multiple links of any type as stream source.

Available source types:

- [rtsp](#source-rtsp) - most cameras on market
- [rtmp](#source-rtmp)
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [exec](#source-exec) - advanced FFmpeg and GStreamer integration
- [hass](#source-hass) - Home Assistant integration

**PS.** You can use sources like `MJPEG`, `HLS` and others via FFmpeg integration.

#### Source: RTSP

- Support **RTSP and RTSPS** links with multiple video and audio tracks
- Support **2-way audio** ONLY for [ONVIF Profile T](https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf) cameras (back channel connection)

**Attention:** other 2-way audio standards are not supported! ONVIF without Profile T is not supported!

```yaml
streams:
  sonoff_camera: rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
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

- FFmpeg preistalled for **Docker** and **Hass Add-on** users
- **Hass Add-on** users can target files from [/media](https://www.home-assistant.io/more-info/local-media/setup-media/) folder

Format: `ffmpeg:{input}#{param1}#{param2}#{param3}`. Examples:

```yaml
streams:
  # [FILE] all tracks will be copied without transcoding codecs
  file1: ffmpeg:/media/BigBuckBunny.mp4

  # [FILE] video will be transcoded to H264, audio will be skipped
  file2: ffmpeg:/media/BigBuckBunny.mp4#video=h264

  # [FILE] video will be copied, audio will be transcoded to pcmu
  file3: ffmpeg:/media/BigBuckBunny.mp4#video=copy#audio=pcmu

  # [HLS] video will be copied, audio will be skipped
  hls: ffmpeg:https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear5/prog_index.m3u8#video=copy

  # [MJPEG] video will be transcoded to H264
  mjpeg: ffmpeg:http://185.97.122.128/cgi-bin/faststream.jpg?stream=half&fps=15#video=h264

  # [RTSP] video and audio will be copied
  rtsp: ffmpeg:rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0#video=copy#audio=copy
```

All trascoding formats has [built-in templates](https://github.com/AlexxIT/go2rtc/blob/master/cmd/ffmpeg/ffmpeg.go): `h264`, `h264/ultra`, `h264/high`, `h265`, `opus`, `pcmu`, `pcmu/16000`, `pcmu/48000`, `pcma`, `pcma/16000`, `pcma/48000`, `aac/16000`.

But you can override them via YAML config. You can also add your own formats to config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg                                        # path to ffmpeg binary
  h264: "-codec:v libx264 -g 30 -preset superfast -tune zerolatency -profile main -level 4.1"
  mycodec: "-any args that support ffmpeg..."
```

#### Source: Exec

FFmpeg source just a shortcut to exec source. You can get any stream or file or device via FFmpeg or GStreamer and push it to go2rtc via RTSP protocol: 

```yaml
streams:
  stream1: exec:ffmpeg -hide_banner -re -stream_loop -1 -i /media/BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp {output}
```

#### Source: Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- support ONLY [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI

```yaml
hass:
  config: "/config"  # skip this setting if you Hass Add-on user

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
```

### Module: API

The HTTP API is the main part for interacting with the application. Default address: `http://127.0.0.1:1984/`.

- you can use WebRTC only when HTTP API enabled
- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "127.0.0.1:1984"` setting
- you can change API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`

```yaml
api:
  listen: ":1984"  # HTTP API port ("" - disabled)
  base_path: ""    # API prefix for serve on suburl
  static_dir: ""   # folder for static files (custom web interface)
```

**PS. go2rtc** don't provide HTTPS or password protection. Use [Nginx](https://nginx.org/) or [Ngrok](#module-ngrok) or [Home Assistant Add-on](#go2rtc-home-assistant-add-on) for this tasks.

**PS2.** You can access microphone (for 2-way audio) only with HTTPS

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
  listen: ":8554"
```

### Module: WebRTC

WebRTC usually works without problems in the local network. But external access may require additional settings. It depends on what type of Internet do you have.

- by default, WebRTC use two random UDP ports for each connection (video and audio)
- you can enable one additional TCP port for all connections and use it for external access

**Static public IP**

- add some TCP port to YAML config (ex. 8555)
- forward this port on your router (you can use same 8555 port or any other)
- add your external IP-address and external port to YAML config

```yaml
webrtc:
  listen: ":8555"  # address of your local server (TCP)
  candidates:
    - 216.58.210.174:8555  # if you have static public IP-address
```

**Dynamic public IP**

- add some TCP port to YAML config (ex. 8555)
- forward this port on your router (you can use same 8555 port or any other)
- add `stun` word and external port to YAML config
  - go2rtc automatically detects your external address with STUN-server

```yaml
webrtc:
  listen: ":8555"  # address of your local server (TCP)
  candidates:
    - stun:8555  # if you have dynamic public IP-address
```

**Private IP**

- add some TCP port to YAML config (ex. 8555)
- setup integration with [Ngrok service](#module-ngrok)

```yaml
webrtc:
  listen: ":8555"  # address of your local server (TCP)

ngrok:
  command: ...
```

**Own TCP-tunnel**

If you have personal VPS, you can create TCP-tunnel and setup in the same way as "Static public IP". But use your VPS IP-address in YAML config.

**Using TURN-server**

TODO...

```yaml
webrtc:
  ice_servers:
    - urls: [stun:stun.l.google.com:19302]
    - urls: [turn:123.123.123.123:3478]
      username: your_user
      credential: your_pass
```

### Module: Ngrok

With Ngrok integration you can get external access to your streams in situation when you have Internet with private IP-address.

- Ngrok preistalled for **Docker** and **Hass Add-on** users
- you may need external access for two different things:
  - WebRTC stream, so you need tunnel WebRTC TCP port (ex. 8555)
  - go2rtc web interface, so you need tunnel API HTTP port (ex. 1984)
- Ngrok support authorization for your web interface
- Ngrok automatically adds HTTPS to your web interface

Ngrok free subscription limitations:

- you will always get random external address (not a problem for webrtc stream)
- you can forward multiple ports but use only one Ngrok app

go2rtc will automatically get your external TCP address (if you enable it in ngrok config) and use it with WebRTC connection (if you enable it in webrtc config).

You need manually download [Ngrok agent app](https://ngrok.com/download) for your OS and register in [Ngrok service](https://ngrok.com/).

**Tunnel for only WebRTC Stream**

You need to add your [Ngrok token](https://dashboard.ngrok.com/get-started/your-authtoken) and WebRTC TCP port to YAML:

```yaml
ngrok:
  command: ngrok tcp 8555 --authtoken eW91IHNoYWxsIG5vdCBwYXNzCnlvdSBzaGFsbCBub3QgcGFzcw
```

**Tunnel for WebRTC and Web interface**

You need to create `ngrok.yaml` config file and add it to go2rtc config:

```yaml
ngrok:
  command: ngrok start --all --config ngrok.yaml
```

Ngrok config example:

```yaml
version: "2"
authtoken: eW91IHNoYWxsIG5vdCBwYXNzCnlvdSBzaGFsbCBub3QgcGFzcw
tunnels:
  api:
    addr: 1984  # use the same port as in go2rtc config
    proto: http
    basic_auth:
      - admin:password  # you can set login/pass for your web interface
  webrtc:
    addr: 8555  # use the same port as in go2rtc config
    proto: tcp
```

### Module: Hass

**go2rtc** compatible with Home Assistant [RTSPtoWebRTC](https://www.home-assistant.io/integrations/rtsp_to_webrtc/) integration.

If you install **go2rtc** as [Hass Add-on](#go2rtc-home-assistant-add-on) - you need to use localhost IP-address, example:

- `http://127.0.0.1:1984/` to web interface
- `rtsp://127.0.0.1:8554/camera1` to RTSP streams

In other cases you need to use IP-address of server with **go2rtc** application.

1. Add integration with link to go2rtc HTTP API:
   - Hass > Settings > Integrations > Add Integration > [RTSPtoWebRTC](https://my.home-assistant.io/redirect/config_flow_start/?domain=rtsp_to_webrtc) > `http://127.0.0.1:1984/`
2. Add generic camera with RTSP link:
   - Hass > Settings > Integrations > Add Integration > [Generic Camera](https://my.home-assistant.io/redirect/config_flow_start/?domain=generic) > `rtsp://...` or `rtmp://...`
   - you can use either direct RTSP links to cameras or take RTSP streams from **go2rtc**
3. Use Picture Entity or Picture Glance lovelace card
4. Open full screen card - this is should be WebRTC stream

PS. Default Home Assistant lovelace cards don't support 2-way audio. You can use 2-way audio from [Add-on Web UI](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons). But you need use HTTPS to access the microphone. This is a browser restriction and cannot be avoided.

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

## Security

By default `go2rtc` start Web interface on port `1984` and RTSP on port `8554`. Both ports are accessible from your local network. So anyone on your local network can watch video from your cameras without authorization. The same rule applies to the Home Assistant Add-on.

This is not a problem if you trust your local network as much as I do. But you can change this behaviour with a `go2rtc.yaml` config:

```yaml
api:
  listen: "127.0.0.1:1984" # localhost

rtsp:
  listen: "127.0.0.1:8554" # localhost

webrtc:
  listen: ":8555" # external TCP port
```

- local access to RTSP is not a problem for [FFmpeg](#source-ffmpeg) integration, because it runs locally on your server
- local access to API is not a problem for [Home Assistant Add-on](#go2rtc-home-assistant-add-on), because Hass runs locally on same server and Add-on Web UI protected with Hass authorization ([Ingress feature](https://www.home-assistant.io/blog/2019/04/15/hassio-ingress/))
- external access to WebRTC TCP port is not a problem, because it used only for transmit encrypted media data
  - anyway you need to open this port to your local network and to the Internet in order for WebRTC to work

If you need Web interface protection without Home Assistant Add-on - you need to use reverse proxy, like [Nginx](https://nginx.org/), [Caddy](https://caddyserver.com/), [Ngrok](https://ngrok.com/), etc.

PS. Additionally WebRTC opens a lot of random UDP ports for transmit encrypted media. They work without problems on the local network. And sometimes work for external access, even if you haven't opened ports on your router. But for stable external WebRTC access, you need to configure the TCP port.

## FAQ

**Q. What's the difference between go2rtc, WebRTC Camera and RTSPtoWebRTC?**

**go2rtc** is a new version of the server-side [WebRTC Camera](https://github.com/AlexxIT/WebRTC) integration, completely rewritten from scratch, with a number of fixes and a huge number of new features. It is compatible with native Home Assistant [RTSPtoWebRTC](https://www.home-assistant.io/integrations/rtsp_to_webrtc/) integration. So you [can use](#module-hass) default lovelace Picture Entity or Picture Glance.

**Q. Why go2rtc is an addon and not an integration?**

Because **go2rtc** is more than just viewing your stream online with WebRTC. You can use it all the time for your various tasks. But every time the Hass is rebooted - all integrations are also rebooted. So your streams may be interrupted if you use them in additional tasks.

When **go2rtc** is released, the **WebRTC Camera** integration will be updated. And you can decide whether to use the integration or the addon.

**Q. Which RTSP link should I use inside Hass?**

You can use direct link to your cameras there (as you always do). **go2rtc** support zero-config feature. You may leave `streams` config section empty. And your streams will be created on the fly on first start from Hass. And your cameras will have multiple connections. Some from Hass directly and one from **go2rtc**.

Also you can specify your streams in **go2rtc** [config file](#configuration) and use RTSP links to this addon. With additional features: multi-source [codecs negotiation](#codecs-negotiation) or FFmpeg [transcoding](#source-ffmpeg) for unsupported codecs. Or use them as source for Frigate. And your cameras will have one connection from **go2rtc**. And **go2rtc** will have multiple connection - some from Hass via RTSP protocol, some from your browser via WebRTC protocol.

Use any config what you like.

**Q. What about lovelace card with support 2-way audio?**

At this moment I am focused on improving stability and adding new features to **go2rtc**. Maybe someone could write such a card themselves. It's not difficult, I have [some sketches](https://github.com/AlexxIT/go2rtc/blob/master/www/webrtc.html).
