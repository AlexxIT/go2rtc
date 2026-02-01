<h1 align="center">

  ![go2rtc](assets/logo.gif)
  <br>
  [![stars](https://img.shields.io/github/stars/AlexxIT/go2rtc?style=flat-square&logo=github)](https://github.com/AlexxIT/go2rtc/stargazers)
  [![docker pulls](https://img.shields.io/docker/pulls/alexxit/go2rtc?style=flat-square&logo=docker&logoColor=white&label=pulls)](https://hub.docker.com/r/alexxit/go2rtc)
  [![releases](https://img.shields.io/github/downloads/AlexxIT/go2rtc/total?color=blue&style=flat-square&logo=github)](https://github.com/AlexxIT/go2rtc/releases)
  [![goreport](https://goreportcard.com/badge/github.com/AlexxIT/go2rtc)](https://goreportcard.com/report/github.com/AlexxIT/go2rtc)
</h1>

Ultimate camera streaming application with support for RTSP, WebRTC, HomeKit, FFmpeg, RTMP, etc.

![go2rtc overview](assets/go2rtc.png)

- zero-dependency and zero-config [small app](#go2rtc-binary) for all OS (Windows, macOS, Linux, ARM)
- zero-delay for many supported protocols (lowest possible streaming latency)
- streaming from [RTSP](#source-rtsp), [RTMP](#source-rtmp), [DVRIP](#source-dvrip), [HTTP](#source-http) (FLV/MJPEG/JPEG/TS), [USB Cameras](#source-ffmpeg-device) and [other sources](#module-streams)
- streaming from any sources, supported by [FFmpeg](#source-ffmpeg)
- streaming to [RTSP](#module-rtsp), [WebRTC](#module-webrtc), [MSE/MP4](#module-mp4), [HomeKit](#module-homekit) [HLS](#module-hls) or [MJPEG](#module-mjpeg)
- [publish](#publish-stream) any source to popular streaming services (YouTube, Telegram, etc.)
- first project in the World with support streaming from [HomeKit Cameras](#source-homekit)
- on-the-fly transcoding for unsupported codecs via [FFmpeg](#source-ffmpeg)
- play audio files and live streams on some cameras with [speaker](#stream-to-camera)
- multi-source two-way [codecs negotiation](#codecs-negotiation)
  - mixing tracks from different sources to single stream
  - auto-match client-supported codecs
  - [two-way audio](#two-way-audio) for some cameras
- can be [integrated to](#module-api) any smart home platform or be used as [standalone app](#go2rtc-binary)

**Supported Formats** - describes the communication API: authorization, encryption, command set, structure of media packets

- devices: `alsa` (Linux audio), `v4l2` (Linux video)
- files: `adts`, `flv`, `h264`, `hevc`, `hls`, `mjpeg`, `mpegts`, `mp4`, `wav`
- network (public and well known): `mpjpeg`, `onvif`, `rtmp`, `rtp`, `rtsp`, `webrtc`, `yuv4mpegpipe`
- network (private and exclusive): `bubble`, `doorbird`, `dvrip`, `eseecloud`, `gopro`, `hass` (Home Assistant), `homekit` (Apple), `isapi` (Hikvision), `kasa` (TP-Link), `multitrans` (TP-Link), `nest` (Google), `ring`, `roborock`, `tapo` and `vigi` (TP-Link), `tuya`, `webtorrent`, `wyze`, `xiaomi` (Mi Home)
- webrtc related: `creality`, `kinesis` (Amazon), `openipc`, `switchbot`, `whep`, `whip`, `wyze`
- other: `ascii`, `echo`, `exec`, `expr`, `ffmpeg`

**Supported Protocols** - describes the transport for data transmission

- public: `http`, `pipe`, `rtmp`, `rtsp`, `tcp`, `udp`, `webrtc`, `ws` (WebSocket)
- private: `cs2` (PPPP), `hap` and `hds` (HomeKit), `tutk` (P2P)

**Inspired by:**

- series of streaming projects from [@deepch](https://github.com/deepch)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [rtsp-simple-server](https://github.com/aler9/rtsp-simple-server) idea from [@aler9](https://github.com/aler9)
- [GStreamer](https://gstreamer.freedesktop.org/) framework pipeline idea
- [MediaSoup](https://mediasoup.org/) framework routing idea
- HomeKit Accessory Protocol from [@brutella](https://github.com/brutella/hap)
- creator of the project's logo [@v_novoseltsev](https://www.instagram.com/v_novoseltsev)

> [!CAUTION]
> The official website of the project is this GitHub repository and go2rtc.org (hosted on GitHub Pages). The website go2rtc[.]com is in no way associated with the authors of this project.

---

- [Fast start](#fast-start)
  - [go2rtc: Binary](#go2rtc-binary)
  - [go2rtc: Docker](#go2rtc-docker)
  - [go2rtc: Home Assistant add-on](#go2rtc-home-assistant-add-on)
  - [go2rtc: Home Assistant integration](#go2rtc-home-assistant-integration)
  - [go2rtc: Dev version](#go2rtc-dev-version)
- [Configuration](#configuration)
  - [Module: Streams](#module-streams)
  - [Two-way audio](#two-way-audio)
  - [Source: RTSP](#source-rtsp)
  - [Source: RTMP](#source-rtmp)
  - [Source: HTTP](#source-http)
  - [Source: ONVIF](#source-onvif)
  - [Source: FFmpeg](#source-ffmpeg)
  - [Source: FFmpeg Device](#source-ffmpeg-device)
  - [Source: Exec](#source-exec)
  - [Source: Echo](#source-echo)
  - [Source: Expr](#source-expr)
  - [Source: HomeKit](#source-homekit)
  - [Source: Bubble](#source-bubble)
  - [Source: DVRIP](#source-dvrip)
  - [Source: Tapo](#source-tapo)
  - [Source: Kasa](#source-kasa)
  - [Source: Multitrans](#source-multitrans)
  - [Source: Tuya](#source-tuya)
  - [Source: Xiaomi](#source-xiaomi)
  - [Source: Wyze](#source-wyze)
  - [Source: GoPro](#source-gopro)
  - [Source: Ivideon](#source-ivideon)
  - [Source: Hass](#source-hass)
  - [Source: ISAPI](#source-isapi)
  - [Source: Nest](#source-nest)
  - [Source: Ring](#source-ring)
  - [Source: Roborock](#source-roborock)
  - [Source: Doorbird](#source-doorbird)
  - [Source: WebRTC](#source-webrtc)
  - [Source: WebTorrent](#source-webtorrent)
  - [Incoming sources](#incoming-sources)
  - [Stream to camera](#stream-to-camera)
  - [Publish stream](#publish-stream)
  - [Preload stream](#preload-stream)
  - [Module: API](#module-api)
  - [Module: RTSP](#module-rtsp)
  - [Module: RTMP](#module-rtmp)
  - [Module: WebRTC](#module-webrtc)
  - [Module: HomeKit](#module-homekit)
  - [Module: WebTorrent](#module-webtorrent)
  - [Module: ngrok](#module-ngrok)
  - [Module: Hass](#module-hass)
  - [Module: MP4](#module-mp4)
  - [Module: HLS](#module-hls)
  - [Module: MJPEG](#module-mjpeg)
  - [Module: Log](#module-log)
- [Security](#security)
- [Codecs filters](#codecs-filters)
- [Codecs madness](#codecs-madness)
- [Codecs negotiation](#codecs-negotiation)
- [Projects using go2rtc](#projects-using-go2rtc)
- [Camera experience](#camera-experience)
- [Tips](#tips)

# Fast start

1. Download [binary](#go2rtc-binary) or use [Docker](#go2rtc-docker) or Home Assistant [add-on](#go2rtc-home-assistant-add-on) or [Integration](#go2rtc-home-assistant-integration)
2. Open web interface: `http://localhost:1984/`

**Optionally:**

- add your [streams](#module-streams) to [config](#configuration) file
- setup [external access](#module-webrtc) to webrtc

**Developers:**

- write your own [web interface](#module-api)
- integrate [web api](#module-api) into your smart home platform

## go2rtc: Binary

Download binary for your OS from [latest release](https://github.com/AlexxIT/go2rtc/releases/):

- `go2rtc_win64.zip` - Windows 10+ 64-bit
- `go2rtc_win32.zip` - Windows 10+ 32-bit
- `go2rtc_win_arm64.zip` - Windows ARM 64-bit
- `go2rtc_linux_amd64` - Linux 64-bit
- `go2rtc_linux_i386` - Linux 32-bit
- `go2rtc_linux_arm64` - Linux ARM 64-bit (ex. Raspberry 64-bit OS)
- `go2rtc_linux_arm` - Linux ARM 32-bit (ex. Raspberry 32-bit OS)
- `go2rtc_linux_armv6` - Linux ARMv6 (for old Raspberry 1 and Zero)
- `go2rtc_linux_mipsel` - Linux MIPS (ex. [Xiaomi Gateway 3](https://github.com/AlexxIT/XiaomiGateway3), [Wyze cameras](https://github.com/gtxaspec/wz_mini_hacks))
- `go2rtc_mac_amd64.zip` - macOS 11+ Intel 64-bit
- `go2rtc_mac_arm64.zip` - macOS ARM 64-bit
- `go2rtc_freebsd_amd64.zip` - FreeBSD 64-bit
- `go2rtc_freebsd_arm64.zip` - FreeBSD ARM 64-bit

Don't forget to fix the rights `chmod +x go2rtc_xxx_xxx` on Linux and Mac.

PS. The application is compiled with the latest versions of the Go language for maximum speed and security. Therefore, the [minimum OS versions](https://go.dev/wiki/MinimumRequirements) depend on the Go language.

## go2rtc: Docker

The Docker container [`alexxit/go2rtc`](https://hub.docker.com/r/alexxit/go2rtc) supports multiple architectures including `amd64`, `386`, `arm64`, and `arm`. This container offers the same functionality as the [Home Assistant add-on](#go2rtc-home-assistant-add-on) but is designed to operate independently of Home Assistant. It comes preinstalled with [FFmpeg](#source-ffmpeg) and [Python](#source-echo).

## go2rtc: Home Assistant add-on

[![Open your Home Assistant instance and show the add add-on repository dialog with a specific repository URL pre-filled.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons)

1. Install Add-On:
    - Settings > Add-ons > Plus > Repositories > Add `https://github.com/AlexxIT/hassio-addons`
    - go2rtc > Install > Start
2. Setup [Integration](#module-hass)

## go2rtc: Home Assistant Integration

[WebRTC Camera](https://github.com/AlexxIT/WebRTC) custom component can be used on any [Home Assistant installation](https://www.home-assistant.io/installation/), including [HassWP](https://github.com/AlexxIT/HassWP) on Windows. It can automatically download and use the latest version of go2rtc. Or it can connect to an existing version of go2rtc. Addon installation in this case is optional.

## go2rtc: Dev version

Latest, but maybe unstable version:

- Binary: [latest nightly release](https://nightly.link/AlexxIT/go2rtc/workflows/build/master)
- Pull Requests: each PR build publishes artifacts, and a bot comment in the PR includes binary download links via nightly.link
- Docker: `alexxit/go2rtc:master` or `alexxit/go2rtc:master-hardware` versions
- Home Assistant add-on: `go2rtc master` or `go2rtc master hardware` versions

# Configuration

- by default go2rtc will search `go2rtc.yaml` in the current work directory
- `api` server will start on default **1984 port** (TCP)
- `rtsp` server will start on default **8554 port** (TCP)
- `webrtc` will use port **8555** (TCP/UDP) for connections
- `ffmpeg` will use default transcoding options

Configuration options and a complete list of settings can be found in [the wiki](https://github.com/AlexxIT/go2rtc/wiki/Configuration).

Available modules:

- [streams](#module-streams)
- [api](#module-api) - HTTP API (important for WebRTC support)
- [rtsp](#module-rtsp) - RTSP Server (important for FFmpeg support)
- [webrtc](#module-webrtc) - WebRTC Server
- [mp4](#module-mp4) - MSE, MP4 stream and MP4 snapshot Server
- [hls](#module-hls) - HLS TS or fMP4 stream Server
- [mjpeg](#module-mjpeg) - MJPEG Server
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [ngrok](#module-ngrok) - ngrok integration (external access for private network)
- [hass](#module-hass) - Home Assistant integration
- [log](#module-log) - logs config

## Module: Streams

**go2rtc** supports different stream source types. You can config one or multiple links of any type as a stream source.

Available source types:

- [rtsp](#source-rtsp) - `RTSP` and `RTSPS` cameras with [two-way audio](#two-way-audio) support
- [rtmp](#source-rtmp) - `RTMP` streams
- [http](#source-http) - `HTTP-FLV`, `MPEG-TS`, `JPEG` (snapshots), `MJPEG` streams
- [onvif](#source-onvif) - get camera `RTSP` link and snapshot link using `ONVIF` protocol
- [ffmpeg](#source-ffmpeg) - FFmpeg integration (`HLS`, `files` and many others)
- [ffmpeg:device](#source-ffmpeg-device) - local USB Camera or Webcam
- [exec](#source-exec) - get media from external app output
- [echo](#source-echo) - get stream link from bash or python
- [expr](#source-expr) - get stream link via built-in expression language
- [homekit](#source-homekit) - streaming from HomeKit Camera
- [bubble](#source-bubble) - streaming from ESeeCloud/dvr163 NVR
- [dvrip](#source-dvrip) - streaming from DVR-IP NVR
- [eseecloud](#source-eseecloud) - streaming from ESeeCloud/dvr163 NVR
- [tapo](#source-tapo) - TP-Link Tapo cameras with [two-way audio](#two-way-audio) support
- [ring](#source-ring) - Ring cameras with [two-way audio](#two-way-audio) support
- [tuya](#source-tuya) - Tuya cameras with [two-way audio](#two-way-audio) support
- [xiaomi](#source-xiaomi) - Xiaomi cameras with [two-way audio](#two-way-audio) support
- [kasa](#source-tapo) - TP-Link Kasa cameras
- [gopro](#source-gopro) - GoPro cameras
- [ivideon](#source-ivideon) - public cameras from [Ivideon](https://tv.ivideon.com/) service
- [hass](#source-hass) - Home Assistant integration
- [isapi](#source-isapi) - two-way audio for Hikvision (ISAPI) cameras
- [roborock](#source-roborock) - Roborock vacuums with cameras
- [doorbird](#source-doorbird) - Doorbird cameras with [two-way audio](#two-way-audio) support
- [webrtc](#source-webrtc) - WebRTC/WHEP sources
- [webtorrent](#source-webtorrent) - WebTorrent source from another go2rtc
- [wyze](#source-wyze) - Wyze cameras with [two-way audio](#two-way-audio) support

Read more about [incoming sources](#incoming-sources)

## Two-way audio

Supported sources:

- [RTSP cameras](#source-rtsp) with [ONVIF Profile T](https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf) (back channel connection)
- [DVRIP](#source-dvrip) cameras
- [TP-Link Tapo](#source-tapo) cameras
- [Hikvision ISAPI](#source-isapi) cameras
- [Roborock vacuums](#source-roborock) models with cameras
- [Doorbird](#source-doorbird) cameras
- [Exec](#source-exec) audio on server
- [Ring](#source-ring) cameras
- [Tuya](#source-tuya) cameras
- [Wyze](#source-wyze) cameras
- [Xiaomi](#source-xiaomi) cameras
- [Any Browser](#incoming-browser) as IP-camera

Two-way audio can be used in browser with [WebRTC](#module-webrtc) technology. The browser will give access to the microphone only for HTTPS sites ([read more](https://stackoverflow.com/questions/52759992/how-to-access-camera-and-microphone-in-chrome-without-https)).

go2rtc also supports [play audio](#stream-to-camera) files and live streams on this cameras.

## Source: RTSP

```yaml
streams:
  sonoff_camera: rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
  dahua_camera:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=1#backchannel=0
  amcrest_doorbell:
    - rtsp://username:password@192.168.1.123:554/cam/realmonitor?channel=1&subtype=0#backchannel=0
  unifi_camera: rtspx://192.168.1.123:7441/fD6ouM72bWoFijxK
  glichy_camera: ffmpeg:rtsp://username:password@192.168.1.123/live/ch00_1 
```

**Recommendations**

- **Amcrest Doorbell** users may want to disable two-way audio, because with an active stream, you won't have a working call button. You need to add `#backchannel=0` to the end of your RTSP link in YAML config file
- **Dahua Doorbell** users may want to change [audio codec](https://github.com/AlexxIT/go2rtc/issues/49#issuecomment-2127107379) for proper two-way audio. Make sure not to request backchannel multiple times by adding `#backchannel=0` to other stream sources of the same doorbell. The `unicast=true&proto=Onvif` is preferred for two-way audio as this makes the doorbell accept multiple codecs for the incoming audio
- **Reolink** users may want NOT to use RTSP protocol at all, some camera models have a very awful, unusable stream implementation
- **Ubiquiti UniFi** users may want to disable HTTPS verification. Use `rtspx://` prefix instead of `rtsps://`. And don't use `?enableSrtp` [suffix](https://github.com/AlexxIT/go2rtc/issues/81)
- **TP-Link Tapo** users may skip login and password, because go2rtc supports login [without them](https://drmnsamoliu.github.io/video.html)
- If your camera has two RTSP links, you can add both as sources. This is useful when streams have different codecs, for example AAC audio with main stream and PCMU/PCMA audio with second stream
- If the stream from your camera is glitchy, try using [ffmpeg source](#source-ffmpeg). It will not add CPU load if you don't use transcoding
- If the stream from your camera is very glitchy, try to use transcoding with [ffmpeg source](#source-ffmpeg)

**Other options**

Format: `rtsp...#{param1}#{param2}#{param3}`

- Add custom timeout `#timeout=30` (in seconds)
- Ignore audio - `#media=video` or ignore video - `#media=audio`
- Ignore two-way audio API `#backchannel=0` - important for some glitchy cameras
- Use WebSocket transport `#transport=ws...`

**RTSP over WebSocket**

```yaml
streams:
  # WebSocket with authorization, RTSP - without
  axis-rtsp-ws:  rtsp://192.168.1.123:4567/axis-media/media.amp?overview=0&camera=1&resolution=1280x720&videoframeskipmode=empty&Axis-Orig-Sw=true#transport=ws://user:pass@192.168.1.123:4567/rtsp-over-websocket
  # WebSocket without authorization, RTSP - with
  dahua-rtsp-ws: rtsp://user:pass@192.168.1.123/cam/realmonitor?channel=1&subtype=1&proto=Private3#transport=ws://192.168.1.123/rtspoverwebsocket
```

## Source: RTMP

You can get a stream from an RTMP server, for example [Nginx with nginx-rtmp-module](https://github.com/arut/nginx-rtmp-module).

```yaml
streams:
  rtmp_stream: rtmp://192.168.1.123/live/camera1
```

## Source: HTTP

Support Content-Type:

- **HTTP-FLV** (`video/x-flv`) - same as RTMP, but over HTTP
- **HTTP-JPEG** (`image/jpeg`) - camera snapshot link, can be converted by go2rtc to MJPEG stream
- **HTTP-MJPEG** (`multipart/x`) - simple MJPEG stream over HTTP
- **MPEG-TS** (`video/mpeg`) - legacy [streaming format](https://en.wikipedia.org/wiki/MPEG_transport_stream)

Source also supports HTTP and TCP streams with autodetection for different formats: **MJPEG**, **H.264/H.265 bitstream**, **MPEG-TS**.

```yaml
streams:
  # [HTTP-FLV] stream in video/x-flv format
  http_flv: http://192.168.1.123:20880/api/camera/stream/780900131155/657617
  
  # [JPEG] snapshots from Dahua camera, will be converted to MJPEG stream
  dahua_snap: http://admin:password@192.168.1.123/cgi-bin/snapshot.cgi?channel=1

  # [MJPEG] stream will be proxied without modification
  http_mjpeg: https://mjpeg.sanford.io/count.mjpeg

  # [MJPEG or H.264/H.265 bitstream or MPEG-TS]
  tcp_magic: tcp://192.168.1.123:12345

  # Add custom header
  custom_header: "https://mjpeg.sanford.io/count.mjpeg#header=Authorization: Bearer XXX"
```

**PS.** Dahua camera has a bug: if you select MJPEG codec for RTSP second stream, snapshot won't work.

## Source: ONVIF

*[New in v1.5.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)*

The source is not very useful if you already know RTSP and snapshot links for your camera. But it can be useful if you don't.

**WebUI > Add** webpage support ONVIF autodiscovery. Your server must be on the same subnet as the camera. If you use Docker, you must use "network host".

```yaml
streams:
  dahua1: onvif://admin:password@192.168.1.123
  reolink1: onvif://admin:password@192.168.1.123:8000
  tapo1: onvif://admin:password@192.168.1.123:2020
```

## Source: FFmpeg

You can get any stream, file or device via FFmpeg and push it to go2rtc. The app will automatically start FFmpeg with the proper arguments when someone starts watching the stream.

- FFmpeg preinstalled for **Docker** and **Home Assistant add-on** users
- **Home Assistant add-on** users can target files from [/media](https://www.home-assistant.io/more-info/local-media/setup-media/) folder

Format: `ffmpeg:{input}#{param1}#{param2}#{param3}`. Examples:

```yaml
streams:
  # [FILE] all tracks will be copied without transcoding codecs
  file1: ffmpeg:/media/BigBuckBunny.mp4

  # [FILE] video will be transcoded to H264, audio will be skipped
  file2: ffmpeg:/media/BigBuckBunny.mp4#video=h264

  # [FILE] video will be copied, audio will be transcoded to PCMU
  file3: ffmpeg:/media/BigBuckBunny.mp4#video=copy#audio=pcmu

  # [HLS] video will be copied, audio will be skipped
  hls: ffmpeg:https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear5/prog_index.m3u8#video=copy

  # [MJPEG] video will be transcoded to H264
  mjpeg: ffmpeg:http://185.97.122.128/cgi-bin/faststream.jpg#video=h264

  # [RTSP] video with rotation, should be transcoded, so select H264
  rotate: ffmpeg:rtsp://12345678@192.168.1.123/av_stream/ch0#video=h264#rotate=90
```

All transcoding formats have [built-in templates](internal/ffmpeg/ffmpeg.go): `h264`, `h265`, `opus`, `pcmu`, `pcmu/16000`, `pcmu/48000`, `pcma`, `pcma/16000`, `pcma/48000`, `aac`, `aac/16000`.

But you can override them via YAML config. You can also add your own formats to the config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg  # path to ffmpeg binary
  global: "-hide_banner"
  timeout: 5  # default timeout in seconds for rtsp inputs
  h264: "-codec:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency -profile:v main -level:v 4.1"
  mycodec: "-any args that supported by ffmpeg..."
  myinput: "-fflags nobuffer -flags low_delay -timeout {timeout} -i {input}"
  myraw: "-ss 00:00:20"
```

- You can use go2rtc stream name as ffmpeg input (ex. `ffmpeg:camera1#video=h264`)
- You can use `video` and `audio` params multiple times (ex. `#video=copy#audio=copy#audio=pcmu`)
- You can use `rotate` param with `90`, `180`, `270` or `-90` values, important with transcoding (ex. `#video=h264#rotate=90`)
- You can use `width` and/or `height` params, important with transcoding (ex. `#video=h264#width=1280`)
- You can use `drawtext` to add a timestamp (ex. `drawtext=x=2:y=2:fontsize=12:fontcolor=white:box=1:boxcolor=black`)
  - This will greatly increase the CPU of the server, even with hardware acceleration
- You can use `timeout` param to set RTSP input timeout in seconds (ex. `#timeout=10`)
- You can use `raw` param for any additional FFmpeg arguments (ex. `#raw=-vf transpose=1`)
- You can use `input` param to override default input template (ex. `#input=rtsp/udp` will change RTSP transport from TCP to UDP+TCP)
  - You can use raw input value (ex. `#input=-timeout {timeout} -i {input}`)
  - You can add your own input templates

Read more about [hardware acceleration](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration).

**PS.** It is recommended to check the available hardware in the WebUI add page.

## Source: FFmpeg Device

You can get video from any USB camera or Webcam as RTSP or WebRTC stream. This is part of FFmpeg integration.

- check available devices in web interface
- `video_size` and `framerate` must be supported by your camera!
- for Linux supported only video for now
- for macOS you can stream FaceTime camera or whole desktop!
- for macOS important to set right framerate

Format: `ffmpeg:device?{input-params}#{param1}#{param2}#{param3}`

```yaml
streams:
  linux_usbcam:   ffmpeg:device?video=0&video_size=1280x720#video=h264
  windows_webcam: ffmpeg:device?video=0#video=h264
  macos_facetime: ffmpeg:device?video=0&audio=1&video_size=1280x720&framerate=30#video=h264#audio=pcma
```

**PS.** It is recommended to check the available devices in the WebUI add page.

## Source: Exec

Exec source can run any external application and expect data from it. Two transports are supported - **pipe** (*from [v1.5.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)*) and **RTSP**.

If you want to use **RTSP** transport, the command must contain the `{output}` argument in any place. On launch, it will be replaced by the local address of the RTSP server.

**pipe** reads data from app stdout in different formats: **MJPEG**, **H.264/H.265 bitstream**, **MPEG-TS**. Also pipe can write data to app stdin in two formats: **PCMA** and **PCM/48000**.

The source can be used with:

- [FFmpeg](https://ffmpeg.org/) - go2rtc ffmpeg source just a shortcut to exec source
- [FFplay](https://ffmpeg.org/ffplay.html) - play audio on your server
- [GStreamer](https://gstreamer.freedesktop.org/)
- [Raspberry Pi Cameras](https://www.raspberrypi.com/documentation/computers/camera_software.html)
- any of your own software

Pipe commands support parameters (format: `exec:{command}#{param1}#{param2}`):

- `killsignal` - signal which will be sent to stop the process (numeric form)
- `killtimeout` - time in seconds for forced termination with sigkill
- `backchannel` - enable backchannel for two-way audio
- `starttimeout` - time in seconds for waiting first byte from RTSP

```yaml
streams:
  stream: exec:ffmpeg -re -i /media/BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp {output}
  picam_h264: exec:libcamera-vid -t 0 --inline -o -
  picam_mjpeg: exec:libcamera-vid -t 0 --codec mjpeg -o -
  pi5cam_h264: exec:libcamera-vid -t 0 --libav-format h264 -o -
  canon: exec:gphoto2 --capture-movie --stdout#killsignal=2#killtimeout=5
  play_pcma: exec:ffplay -fflags nobuffer -f alaw -ar 8000 -i -#backchannel=1
  play_pcm48k: exec:ffplay -fflags nobuffer -f s16be -ar 48000 -i -#backchannel=1
```

## Source: Echo

Some sources may have a dynamic link. And you will need to get it using a Bash or Python script. Your script should echo a link to the source. RTSP, FFmpeg or any of the [supported sources](#module-streams).

**Docker** and **Home Assistant add-on** users has preinstalled `python3`, `curl`, `jq`.

Check examples in [wiki](https://github.com/AlexxIT/go2rtc/wiki/Source-Echo-examples).

```yaml
streams:
  apple_hls: echo:python3 hls.py https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html
```

## Source: Expr

*[New in v1.8.2](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.2)*

Like `echo` source, but uses the built-in [expr](https://github.com/antonmedv/expr) expression language.

*[read more](internal/expr/README.md)*

## Source: HomeKit

**Important:**

- You can use HomeKit Cameras **without Apple devices** (iPhone, iPad, etc.), it's just a yet another protocol
- HomeKit device can be paired with only one ecosystem. So, if you have paired it to an iPhone (Apple Home), you can't pair it with Home Assistant or go2rtc. Or if you have paired it to go2rtc, you can't pair it with an iPhone
- HomeKit device should be on the same network with working [mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) between the device and go2rtc

go2rtc supports importing paired HomeKit devices from [Home Assistant](#source-hass). So you can use HomeKit camera with Home Assistant and go2rtc simultaneously. If you are using Home Assistant, I recommend pairing devices with it; it will give you more options.

You can pair device with go2rtc on the HomeKit page. If you can't see your devices, reload the page. Also, try rebooting your HomeKit device (power off). If you still can't see it, you have a problem with mDNS.

If you see a device but it does not have a pairing button, it is paired to some ecosystem (Apple Home, Home Assistant, HomeBridge etc). You need to delete the device from that ecosystem, and it will be available for pairing. If you cannot unpair the device, you will have to reset it.

**Important:**

- HomeKit audio uses very non-standard **AAC-ELD** codec with very non-standard params and specification violations
- Audio can't be played in `VLC` and probably any other player
- Audio should be transcoded for use with MSE, WebRTC, etc.

Recommended settings for using HomeKit Camera with WebRTC, MSE, MP4, RTSP:

```yaml
streams:
  aqara_g3:
    - hass:Camera-Hub-G3-AB12
    - ffmpeg:aqara_g3#audio=aac#audio=opus
```

RTSP link with "normal" audio for any player: `rtsp://192.168.1.123:8554/aqara_g3?video&audio=aac`

**This source is in active development!** Tested only with [Aqara Camera Hub G3](https://www.aqara.com/eu/product/camera-hub-g3) (both EU and CN versions).

## Source: Bubble

*[New in v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*

Other names: [ESeeCloud](http://www.eseecloud.com/), [dvr163](http://help.dvr163.com/).

- you can skip `username`, `password`, `port`, `ch` and `stream` if they are default
- set up separate streams for different channels and streams

```yaml
streams:
  camera1: bubble://username:password@192.168.1.123:34567/bubble/live?ch=0&stream=0
```

## Source: DVRIP

*[New in v1.2.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.2.0)*

Other names: DVR-IP, NetSurveillance, Sofia protocol (NETsurveillance ActiveX plugin XMeye SDK).

- you can skip `username`, `password`, `port`, `channel` and `subtype` if they are default
- set up separate streams for different channels
- use `subtype=0` for Main stream, and `subtype=1` for Extra1 stream
- only the TCP protocol is supported

```yaml
streams:
  only_stream: dvrip://username:password@192.168.1.123:34567?channel=0&subtype=0
  only_tts: dvrip://username:password@192.168.1.123:34567?backchannel=1
  two_way_audio:
    - dvrip://username:password@192.168.1.123:34567?channel=0&subtype=0
    - dvrip://username:password@192.168.1.123:34567?backchannel=1
```

## Source: EseeCloud

*[New in v1.9.10](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.10)*

```yaml
streams:
  camera1: eseecloud://user:pass@192.168.1.123:80/livestream/12
```

## Source: Tapo

*[New in v1.2.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.2.0)*

[TP-Link Tapo](https://www.tapo.com/) proprietary camera protocol with **two-way audio** support.

- stream quality is the same as [RTSP protocol](https://www.tapo.com/en/faq/34/)
- use the **cloud password**, this is not the RTSP password! you do not need to add a login!
- you can also use **UPPERCASE** MD5 hash from your cloud password with `admin` username
- some new camera firmwares require SHA256 instead of MD5

```yaml
streams:
  # cloud password without username
  camera1: tapo://cloud-password@192.168.1.123
  # admin username and UPPERCASE MD5 cloud-password hash
  camera2: tapo://admin:UPPERCASE-MD5@192.168.1.123
  # admin username and UPPERCASE SHA256 cloud-password hash
  camera3: tapo://admin:UPPERCASE-SHA256@192.168.1.123
  # VGA stream (the so called substream, the lower resolution one)
  camera4: tapo://cloud-password@192.168.1.123?subtype=1 
  # HD stream (default)
  camera5: tapo://cloud-password@192.168.1.123?subtype=0 
```

```bash
echo -n "cloud password" | md5 | awk '{print toupper($0)}'
echo -n "cloud password" | shasum -a 256 | awk '{print toupper($0)}'
```

## Source: Kasa

*[New in v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*

[TP-Link Kasa](https://www.kasasmart.com/) non-standard protocol [more info](https://medium.com/@hu3vjeen/reverse-engineering-tp-link-kc100-bac4641bf1cd).

- `username` - urlsafe email, `alex@gmail.com` -> `alex%40gmail.com`
- `password` - base64password, `secret1` -> `c2VjcmV0MQ==`

```yaml
streams:
  kc401: kasa://username:password@192.168.1.123:19443/https/stream/mixed
```

Tested: KD110, KC200, KC401, KC420WS, EC71.

## Source: Multitrans

Two-way audio support for Chinese version of [TP-Link cameras](https://www.tp-link.com.cn/list_2549.html).

*[read more](internal/multitrans/README.md)*

## Source: Tuya

*[New in v1.9.13](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13)*

[Tuya](https://www.tuya.com/) proprietary camera protocol with **two-way audio** support. Go2rtc supports `Tuya Smart API` and `Tuya Cloud API`.

*[read more](internal/tuya/README.md)*

## Source: Xiaomi

*[New in v1.9.13](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13)*

This source allows you to view cameras from the [Xiaomi Mi Home](https://home.mi.com/) ecosystem.

*[read more](internal/xiaomi/README.md)*

## Source: Wyze

This source allows you to stream from [Wyze](https://wyze.com/) cameras using native P2P protocol - no `docker-wyze-bridge` required. Supports H.264/H.265 video, AAC/G.711 audio, and two-way audio.

*[read more](internal/wyze/README.md)*

## Source: GoPro

*[New in v1.8.3](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.3)*

Support streaming from [GoPro](https://gopro.com/) cameras, connected via USB or Wi-Fi to Linux, Mac, Windows.

*[read more](internal/gopro/README.md)*

## Source: Ivideon

Support public cameras from the service [Ivideon](https://tv.ivideon.com/).

```yaml
streams:
  quailcam: ivideon:100-tu5dkUPct39cTp9oNEN2B6/0
```

## Source: Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI
- [HomeKit Camera](https://www.home-assistant.io/integrations/homekit_controller/)
- [ONVIF](https://www.home-assistant.io/integrations/onvif/)
- [Roborock](https://github.com/humbertogontijo/homeassistant-roborock) vacuums with camera

```yaml
hass:
  config: "/config"  # skip this setting if you are a Home Assistant add-on user

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
  aqara_g3: hass:Camera-Hub-G3-AB12
```

**WebRTC Cameras** (*from [v1.6.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.0)*)

Any cameras in WebRTC format are supported. But at the moment Home Assistant only supports some [Nest](https://www.home-assistant.io/integrations/nest/) cameras in this format.

**Important.** The Nest API only allows you to get a link to a stream for 5 minutes. Do not use this with Frigate! If the stream expires, Frigate will consume all available RAM on your machine within seconds. It's recommended to use [Nest source](#source-nest) - it supports extending the stream.

```yaml
streams:
  # link to Home Assistant Supervised
  hass-webrtc1: hass://supervisor?entity_id=camera.nest_doorbell
  # link to external Home Assistant with Long-Lived Access Tokens
  hass-webrtc2: hass://192.168.1.123:8123?entity_id=camera.nest_doorbell&token=eyXYZ...
```

**RTSP Cameras**

By default, the Home Assistant API does not allow you to get a dynamic RTSP link to a camera stream. [This method](https://github.com/felipecrs/hass-expose-camera-stream-source#importing-cameras-from-home-assistant-to-go2rtc-or-frigate) can work around it.

## Source: ISAPI

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type supports only backchannel audio for the Hikvision ISAPI protocol. So it should be used as a second source in addition to the RTSP protocol.

```yaml
streams:
  hikvision1:
    - rtsp://admin:password@192.168.1.123:554/Streaming/Channels/101
    - isapi://admin:password@192.168.1.123:80/
```

## Source: Nest

*[New in v1.6.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.0)*

Currently, only WebRTC cameras are supported.

For simplicity, it is recommended to connect the Nest/WebRTC camera to the [Home Assistant](#source-hass). But if you can somehow get the below parameters, Nest/WebRTC source will work without Home Assistant.

```yaml
streams:
  nest-doorbell: nest:?client_id=***&client_secret=***&refresh_token=***&project_id=***&device_id=***
```

## Source: Ring

This source type supports Ring cameras with [two-way audio](#two-way-audio) support. If you have a `refresh_token` and `device_id`, you can use them in the `go2rtc.yaml` config file. Otherwise, you can use the go2rtc web interface and add your Ring account (WebUI > Add > Ring). Once added, it will list all your Ring cameras.

```yaml
streams:
  ring: ring:?device_id=XXX&refresh_token=XXX
  ring_snapshot: ring:?device_id=XXX&refresh_token=XXX&snapshot
```

## Source: Roborock

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type supports Roborock vacuums with cameras. Known working models:

- Roborock S6 MaxV - only video (the vacuum has no microphone)
- Roborock S7 MaxV - video and two-way audio
- Roborock Qrevo MaxV - video and two-way audio

This source supports loading Roborock credentials from the Home Assistant [custom integration](https://github.com/humbertogontijo/homeassistant-roborock) or the [core integration](https://www.home-assistant.io/integrations/roborock). Otherwise, you need to log in to your Roborock account (MiHome account is not supported). Go to go2rtc WebUI > Add webpage. Copy the `roborock://...` source for your vacuum and paste it into your `go2rtc.yaml` config.

If you have a pattern PIN for your vacuum, add it as a numeric PIN (lines: 123, 456, 789) to the end of the `roborock` link.

## Source: Doorbird

This source type supports [Doorbird](https://www.doorbird.com/) devices including MJPEG stream, audio stream as well as two-way audio.

*[read more](internal/doorbird/README.md)*

## Source: WebRTC

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type supports four connection formats.

**whep**

[WebRTC/WHEP](https://datatracker.ietf.org/doc/draft-murillo-whep/) is replaced by [WebRTC/WISH](https://datatracker.ietf.org/doc/charter-ietf-wish/02/) standard for WebRTC video/audio viewers. But it may already be supported in some third-party software. It is supported in go2rtc.

**go2rtc**

This format is only supported in go2rtc. Unlike WHEP, it supports asynchronous WebRTC connections and two-way audio.

**openipc** (*from [v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*)

Support connection to [OpenIPC](https://openipc.org/) cameras.

**wyze (via docker-wyze-bridge)** (*from [v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*)

Legacy method to connect to [Wyze](https://www.wyze.com/) cameras using WebRTC protocol via [docker-wyze-bridge](https://github.com/mrlt8/docker-wyze-bridge). For native P2P support without docker-wyze-bridge, see [Source: Wyze](#source-wyze).

**kinesis** (*from [v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*)

Supports [Amazon Kinesis Video Streams](https://aws.amazon.com/kinesis/video-streams/), using WebRTC protocol. You need to specify the signalling WebSocket URL with all credentials in query params, `client_id` and `ice_servers` list in [JSON format](https://developer.mozilla.org/en-US/docs/Web/API/RTCIceServer).

**switchbot**

Support connection to [SwitchBot](https://us.switch-bot.com/) cameras that are based on Kinesis Video Streams. Specifically, this includes [Pan/Tilt Cam Plus 2K](https://us.switch-bot.com/pages/switchbot-pan-tilt-cam-plus-2k) and [Pan/Tilt Cam Plus 3K](https://us.switch-bot.com/pages/switchbot-pan-tilt-cam-plus-3k) and [Smart Video Doorbell](https://www.switchbot.jp/products/switchbot-smart-video-doorbell). `Outdoor Spotlight Cam 1080P`, `Outdoor Spotlight Cam 2K`, `Pan/Tilt Cam`, `Pan/Tilt Cam 2K`, `Indoor Cam` are based on Tuya, so this feature is not available.

```yaml
streams:
  webrtc-whep:      webrtc:http://192.168.1.123:1984/api/webrtc?src=camera1
  webrtc-go2rtc:    webrtc:ws://192.168.1.123:1984/api/ws?src=camera1
  webrtc-openipc:   webrtc:ws://192.168.1.123/webrtc_ws#format=openipc#ice_servers=[{"urls":"stun:stun.kinesisvideo.eu-north-1.amazonaws.com:443"}]
  webrtc-wyze:      webrtc:http://192.168.1.123:5000/signaling/camera1?kvs#format=wyze
  webrtc-kinesis:   webrtc:wss://...amazonaws.com/?...#format=kinesis#client_id=...#ice_servers=[{...},{...}]
  webrtc-switchbot: webrtc:wss://...amazonaws.com/?...#format=switchbot#resolution=hd#play_type=0#client_id=...#ice_servers=[{...},{...}]
```

**PS.** For `kinesis` sources, you can use [echo](#source-echo) to get connection params using `bash`, `python` or any other script language.

## Source: WebTorrent

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source can get a stream from another go2rtc via [WebTorrent](#module-webtorrent) protocol.

```yaml
streams:
  webtorrent1: webtorrent:?share=huofssuxaty00izc&pwd=k3l2j9djeg8v8r7e
```

## Incoming sources

By default, go2rtc establishes a connection to the source when any client requests it. Go2rtc drops the connection to the source when it has no clients left.

- Go2rtc also can accepts incoming sources in [RTSP](#module-rtsp), [RTMP](#module-rtmp), [HTTP](#source-http) and **WebRTC/WHIP** formats
- Go2rtc won't stop such a source if it has no clients
- You can push data only to an existing stream (create a stream with empty source in config)
- You can push multiple incoming sources to the same stream
- You can push data to a non-empty stream, so it will have additional codecs inside

**Examples**

- RTSP with any codec

  ```yaml
  ffmpeg -re -i BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp rtsp://localhost:8554/camera1
  ```

- HTTP-MJPEG with MJPEG codec

  ```yaml
  ffmpeg -re -i BigBuckBunny.mp4 -c mjpeg -f mpjpeg http://localhost:1984/api/stream.mjpeg?dst=camera1
  ```

- HTTP-FLV with H264, AAC codecs

  ```yaml
  ffmpeg -re -i BigBuckBunny.mp4 -c copy -f flv http://localhost:1984/api/stream.flv?dst=camera1
  ```

- MPEG-TS with H264 codec

  ```yaml
  ffmpeg -re -i BigBuckBunny.mp4 -c copy -f mpegts http://localhost:1984/api/stream.ts?dst=camera1
  ```

### Incoming: Browser

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

You can turn the browser of any PC or mobile into an IP camera with support for video and two-way audio. Or even broadcast your PC screen:

1. Create empty stream in the `go2rtc.yaml`
2. Go to go2rtc WebUI
3. Open `links` page for your stream
4. Select `camera+microphone` or `display+speaker` option
5. Open `webrtc` local page (your go2rtc **should work over HTTPS!**) or `share link` via [WebTorrent](#module-webtorrent) technology (work over HTTPS by default)

### Incoming: WebRTC/WHIP

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

You can use **OBS Studio** or any other broadcast software with [WHIP](https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html) protocol support. This standard has not yet been approved. But you can download OBS Studio [dev version](https://github.com/obsproject/obs-studio/actions/runs/3969201209):

- Settings > Stream > Service: WHIP > `http://192.168.1.123:1984/api/webrtc?dst=camera1`

## Stream to camera

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

go2rtc supports playing audio files (ex. music or [TTS](https://www.home-assistant.io/integrations/#text-to-speech)) and live streams (ex. radio) on cameras with [two-way audio](#two-way-audio) support (RTSP/ONVIF cameras, TP-Link Tapo, Hikvision ISAPI, Roborock vacuums, any Browser).

API example:

```text
POST http://localhost:1984/api/streams?dst=camera1&src=ffmpeg:http://example.com/song.mp3#audio=pcma#input=file
```

- you can stream: local files, web files, live streams or any format, supported by FFmpeg
- you should use [ffmpeg source](#source-ffmpeg) for transcoding audio to codec, that your camera supports
- you can check camera codecs on the go2rtc WebUI info page when the stream is active
- some cameras support only low quality `PCMA/8000` codec (ex. [Tapo](#source-tapo))
- it is recommended to choose higher quality formats if your camera supports them (ex. `PCMA/48000` for some Dahua cameras)
- if you play files over `http` link, you need to add `#input=file` params for transcoding, so the file will be transcoded and played in real time
- if you play live streams, you should skip `#input` param, because it is already in real time
- you can stop active playback by calling the API with the empty `src` parameter
- you will see one active producer and one active consumer in go2rtc WebUI info page during streaming

## Publish stream

*[New in v1.8.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)*

You can publish any stream to streaming services (YouTube, Telegram, etc.) via RTMP/RTMPS. Important:

- Supported codecs: H264 for video and AAC for audio
- AAC audio is required for YouTube; videos without audio will not work
- You don't need to enable [RTMP module](#module-rtmp) listening for this task

You can use the API:

```text
POST http://localhost:1984/api/streams?src=camera1&dst=rtmps://...
```

Or config file:

```yaml
publish:
  # publish stream "video_audio_transcode" to Telegram
  video_audio_transcode:
    - rtmps://xxx-x.rtmp.t.me/s/xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx
  # publish stream "audio_transcode" to Telegram and YouTube
  audio_transcode:
    - rtmps://xxx-x.rtmp.t.me/s/xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx
    - rtmp://xxx.rtmp.youtube.com/live2/xxxx-xxxx-xxxx-xxxx-xxxx

streams:
  video_audio_transcode:
    - ffmpeg:rtsp://user:pass@192.168.1.123/stream1#video=h264#hardware#audio=aac
  audio_transcode:
    - ffmpeg:rtsp://user:pass@192.168.1.123/stream1#video=copy#audio=aac
```

- **Telegram Desktop App** > Any public or private channel or group (where you admin) > Live stream > Start with... > Start streaming.
- **YouTube** > Create > Go live > Stream latency: Ultra low-latency > Copy: Stream URL + Stream key.

## Preload stream

You can preload any stream on go2rtc start. This is useful for cameras that take a long time to start up.

```yaml
preload:
  camera1:                                     # default: video&audio = ANY
  camera2: "video"                             # preload only video track
  camera3: "video=h264&audio=opus"             # preload H264 video and OPUS audio

streams:
  camera1: 
    - rtsp://192.168.1.100/stream
  camera2: 
    - rtsp://192.168.1.101/stream  
  camera3: 
    - rtsp://192.168.1.102/h265stream
    - ffmpeg:camera3#video=h264#audio=opus#hardware
```

## Module: API

The HTTP API is the main part for interacting with the application. Default address: `http://localhost:1984/`.

**Important!** go2rtc passes requests from localhost and Unix sockets without HTTP authorization, even if you have it configured. It is your responsibility to set up secure external access to the API. If not properly configured, an attacker can gain access to your cameras and even your server.

[API description](api/README.md).

**Module config**

- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "127.0.0.1:1984"` setting
- you can change the API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`
- you can use raw TLS cert/key content or path to files

```yaml
api:
  listen: ":1984"    # default ":1984", HTTP API port ("" - disabled)
  username: "admin"  # default "", Basic auth for WebUI
  password: "pass"   # default "", Basic auth for WebUI
  local_auth: true   # default false, Enable auth check for localhost requests
  base_path: "/rtc"  # default "", API prefix for serving on suburl (/api => /rtc/api)
  static_dir: "www"  # default "", folder for static files (custom web interface)
  origin: "*"        # default "", allow CORS requests (only * supported)
  tls_listen: ":443" # default "", enable HTTPS server
  tls_cert: |        # default "", PEM-encoded fullchain certificate for HTTPS
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  tls_key: |         # default "", PEM-encoded private key for HTTPS
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
  unix_listen: "/tmp/go2rtc.sock"  # default "", unix socket listener for API
```

**PS:**

- MJPEG over WebSocket plays better than native MJPEG because Chrome [bug](https://bugs.chromium.org/p/chromium/issues/detail?id=527446)
- MP4 over WebSocket was created only for Apple iOS because it doesn't support MSE and native MP4

## Module: RTSP

You can get any stream as RTSP-stream: `rtsp://192.168.1.123:8554/{stream_name}`

You can enable external password protection for your RTSP streams. Password protection is always disabled for localhost calls (ex. FFmpeg or Home Assistant on the same server).

```yaml
rtsp:
  listen: ":8554"    # RTSP Server TCP port, default - 8554
  username: "admin"  # optional, default - disabled
  password: "pass"   # optional, default - disabled
  default_query: "video&audio"  # optional, default codecs filters 
```

By default go2rtc provide RTSP-stream with only one first video and only one first audio. You can change it with the `default_query` setting:

- `default_query: "mp4"` - MP4 compatible codecs (H264, H265, AAC)
- `default_query: "video=all&audio=all"` - all tracks from all source (not all players can handle this)
- `default_query: "video=h264,h265"` - only one video track (H264 or H265)
- `default_query: "video&audio=all"` - only one first any video and all audio as separate tracks

Read more about [codecs filters](#codecs-filters).

## Module: RTMP

*[New in v1.8.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)*

You can get any stream as RTMP-stream: `rtmp://192.168.1.123/{stream_name}`. Only H264/AAC codecs supported right now.

[Incoming stream](#incoming-sources) in RTMP format tested only with [OBS Studio](https://obsproject.com/) and a Dahua camera. Different FFmpeg versions have different problems with this format.

```yaml
rtmp:
  listen: ":1935"  # by default - disabled!
```

## Module: WebRTC

In most cases, [WebRTC](https://en.wikipedia.org/wiki/WebRTC) uses a direct peer-to-peer connection from your browser to go2rtc and sends media data via UDP.
It **can't pass** media data through your Nginx or Cloudflare or [Nabu Casa](https://www.nabucasa.com/) HTTP TCP connection!
It can automatically detect your external IP via a public [STUN](https://en.wikipedia.org/wiki/STUN) server.
It can establish an external direct connection via [UDP hole punching](https://en.wikipedia.org/wiki/UDP_hole_punching) technology even if you do not open your server to the World.

But about 10-20% of users may need to configure additional settings for external access if **mobile phone** or **go2rtc server** is behind [Symmetric NAT](https://tomchen.github.io/symmetric-nat-test/).

- by default, WebRTC uses both TCP and UDP on port 8555 for connections
- you can use this port for external access
- you can change the port in YAML config:

```yaml
webrtc:
  listen: ":8555"  # address of your local server and port (TCP/UDP)
```

**Static public IP**

- forward the port 8555 on your router (you can use the same 8555 port or any other as external port)
- add your external IP address and external port to the YAML config

```yaml
webrtc:
  candidates:
    - 216.58.210.174:8555  # if you have a static public IP address
```

**Dynamic public IP**

- forward the port 8555 on your router (you can use the same 8555 port or any other as the external port)
- add `stun` word and external port to YAML config
  - go2rtc automatically detects your external address with STUN server

```yaml
webrtc:
  candidates:
    - stun:8555  # if you have a dynamic public IP address
```

**Hard tech way 1. Own TCP-tunnel**

If you have a personal [VPS](https://en.wikipedia.org/wiki/Virtual_private_server), you can create a TCP tunnel and setup in the same way as "Static public IP". But use your VPS IP address in the YAML config.

**Hard tech way 2. Using TURN-server**

If you have personal [VPS](https://en.wikipedia.org/wiki/Virtual_private_server), you can install TURN server (e.g. [coturn](https://github.com/coturn/coturn), config [example](https://github.com/AlexxIT/WebRTC/wiki/Coturn-Example)).

```yaml
webrtc:
  ice_servers:
    - urls: [stun:stun.l.google.com:19302]
    - urls: [turn:123.123.123.123:3478]
      username: your_user
      credential: your_pass
```

## Module: HomeKit

*[New in v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*

HomeKit module can work in two modes:

- export any H264 camera to Apple HomeKit
- transparent proxy any Apple HomeKit camera (Aqara, Eve, Eufy, etc.) back to Apple HomeKit, so you will have all camera features in Apple Home and also will have RTSP/WebRTC/MP4/etc. from your HomeKit camera

**Important**

- HomeKit cameras support only H264 video and OPUS audio

**Minimal config**

```yaml
streams:
  dahua1: rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
homekit:
  dahua1:  # same stream ID from streams list, default PIN - 19550224
```

**Full config**

```yaml
streams:
  dahua1:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
    - ffmpeg:dahua1#video=h264#hardware  # if your camera doesn't support H264, important for HomeKit
    - ffmpeg:dahua1#audio=opus           # only OPUS audio supported by HomeKit

homekit:
  dahua1:                   # same stream ID from streams list
    pin: 12345678           # custom PIN, default: 19550224
    name: Dahua camera      # custom camera name, default: generated from stream ID
    device_id: dahua1       # custom ID, default: generated from stream ID
    device_private: dahua1  # custom key, default: generated from stream ID
```

**Proxy HomeKit camera**

- Video stream from HomeKit camera to Apple device (iPhone, AppleTV) will be transmitted directly
- Video stream from HomeKit camera to RTSP/WebRTC/MP4/etc. will be transmitted via go2rtc

```yaml
streams:
  aqara1:
    - homekit://...
    - ffmpeg:aqara1#audio=aac#audio=opus  # optional audio transcoding

homekit:
  aqara1:  # same stream ID from streams list
```

## Module: WebTorrent

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This module supports:

- Share any local stream via [WebTorrent](https://webtorrent.io/) technology
- Get any [incoming stream](#incoming-browser) from PC or mobile via [WebTorrent](https://webtorrent.io/) technology
- Get any remote [go2rtc source](#source-webtorrent) via [WebTorrent](https://webtorrent.io/) technology

Securely and freely. You do not need to open a public access to the go2rtc server. But in some cases (Symmetric NAT), you may need to set up external access to [WebRTC module](#module-webrtc).

To generate a sharing link or incoming link, go to the go2rtc WebUI (stream links page). This link is **temporary** and will stop working after go2rtc is restarted!

You can create permanent external links in the go2rtc config:

```yaml
webtorrent:
  shares:
    super-secret-share:  # share name, should be unique among all go2rtc users!
      pwd: super-secret-password
      src: rtsp-dahua1   # stream name from streams section
```

Link example: `https://go2rtc.org/webtorrent/#share=02SNtgjKXY&pwd=wznEQqznxW&media=video+audio`

## Module: ngrok

With [ngrok](https://ngrok.com/) integration, you can get external access to your streams in situations when you have internet with a private IP address.

*[read more](internal/ngrok/README.md)*

## Module: Hass

While [go2rtc is used by default in Home Assistant](https://www.home-assistant.io/integrations/go2rtc/), the best and easiest way to have full control over it is to install the [WebRTC Camera](#go2rtc-home-assistant-integration) custom integration and card.

You have several options on how to add a camera to Home Assistant:

1. Camera RTSP source => [Generic Camera](https://www.home-assistant.io/integrations/generic/)
2. Camera [any source](#module-streams) => [go2rtc config](#configuration) => [Generic Camera](https://www.home-assistant.io/integrations/generic/)
   - Install any [go2rtc](#fast-start)
   - Add your stream to [go2rtc config](#configuration)
   - Home Assistant > Settings > Integrations > Add Integration > [ONVIF](https://my.home-assistant.io/redirect/config_flow_start/?domain=onvif) > Host: `127.0.0.1`, Port: `1984`
   - Home Assistant > Settings > Integrations > Add Integration > [Generic Camera](https://my.home-assistant.io/redirect/config_flow_start/?domain=generic) > Stream Source URL: `rtsp://127.0.0.1:8554/camera1` (change to your stream name, leave everything else as is)

You have several options on how to watch the stream from the cameras in Home Assistant:

1. `Camera Entity` => `Picture Entity Card` => Technology `WebRTC` (through [built-in go2rtc](https://www.home-assistant.io/integrations/go2rtc/)), codecs `H264/H265/AAC/PCMU/PCMA/OPUS`, best latency. Fallbacks to: Technology `HLS`, codecs: `H264/H265/AAC`, poor latency.
2. `Camera Entity` or `Camera URL` => [WebRTC Camera](https://github.com/AlexxIT/WebRTC) => Technology: `WebRTC/MSE/MP4/MJPEG`, codecs: `H264/H265/AAC/PCMU/PCMA/OPUS`, best latency, best compatibility.
   - Install and add [WebRTC Camera](https://github.com/AlexxIT/WebRTC) custom integration
   - Use WebRTC Camera custom card in your dashboard

You can add camera `entity_id` to [go2rtc config](#configuration) if you need transcoding:

```yaml
streams:
  "camera.hall": ffmpeg:{input}#video=copy#audio=opus
```

**PS.** Default Home Assistant cards don't support two-way audio. You can use two-way audio from the [add-on Web UI](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons), but you need to use HTTPS to access the microphone. This is a browser restriction and cannot be avoided.

**PS.** There is also another nice card with two-way audio support through go2rtc - [Advanced Camera Card](https://github.com/dermotduffy/advanced-camera-card).

## Module: MP4

Provides several features:

1. MSE stream (fMP4 over WebSocket)
2. Camera snapshots in MP4 format (single frame), can be sent to [Telegram](https://github.com/AlexxIT/go2rtc/wiki/Snapshot-to-Telegram)
3. HTTP progressive streaming (MP4 file stream) - bad format for streaming because of high start delay. This format doesn't work in all Safari browsers, but go2rtc will automatically redirect it to HLS/fMP4 in this case.

API examples:

- MP4 snapshot: `http://192.168.1.123:1984/api/frame.mp4?src=camera1` (H264, H265)
- MP4 stream: `http://192.168.1.123:1984/api/stream.mp4?src=camera1` (H264, H265, AAC)
- MP4 file: `http://192.168.1.123:1984/api/stream.mp4?src=camera1` (H264, H265*, AAC, OPUS, MP3, PCMA, PCMU, PCM)
  - You can use `mp4`, `mp4=flac` and `mp4=all` param for codec filters
  - You can use `duration` param in seconds (ex. `duration=15`)
  - You can use `filename` param (ex. `filename=record.mp4`)
  - You can use `rotate` param with `90`, `180` or `270` values
  - You can use `scale` param with positive integer values (ex. `scale=4:3`)

Read more about [codecs filters](#codecs-filters).

**PS.** Rotate and scale params don't use transcoding and change video using metadata.

## Module: HLS

*[New in v1.1.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.1.0)*

[HLS](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) is the worst technology for real-time streaming. It can only be useful on devices that do not support more modern technology, like [WebRTC](#module-webrtc), [MSE/MP4](#module-mp4).

The go2rtc implementation differs from the standards and may not work with all players.

API examples:

- HLS/TS stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1` (H264)
- HLS/fMP4 stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4` (H264, H265, AAC)

Read more about [codecs filters](#codecs-filters).

## Module: MJPEG

- This module can provide and receive streams in MJPEG format.
- This module is also responsible for receiving snapshots in JPEG format.
- This module also supports streaming to the server console (terminal) in the **animated ASCII art** format.

*[read more](internal/mjpeg/README.md)*

## Module: Log

You can set different log levels for different modules.

```yaml
log:
  level: info  # default level
  api: trace
  exec: debug
  rtsp: warn
  streams: error
  webrtc: fatal
```

# Security

> [!IMPORTANT]
> If an attacker gains access to the API, you are in danger. Through the API, an attacker can use insecure sources such as echo and exec. And get full access to your server.

For maximum (paranoid) security, go2rtc has special settings:

```yaml
app:
  # use only allowed modules
  modules: [api, rtsp, webrtc, exec, ffmpeg, mjpeg]

api:
  # use only allowed API paths
  allow_paths: [/api, /api/streams, /api/webrtc, /api/frame.jpeg]
  # enable auth for localhost (used together with username and password)
  local_auth: true

exec:
  # use only allowed exec paths
  allow_paths: [ffmpeg]
```

By default, `go2rtc` starts the Web interface on port `1984` and RTSP on port `8554`, as well as uses port `8555` for WebRTC connections. The three ports are accessible from your local network. So anyone on your local network can watch video from your cameras without authorization. The same rule applies to the Home Assistant add-on.

This is not a problem if you trust your local network as much as I do. But you can change this behaviour with a `go2rtc.yaml` config:

```yaml
api:
  listen: "127.0.0.1:1984" # localhost

rtsp:
  listen: "127.0.0.1:8554" # localhost

webrtc:
  listen: ":8555" # external TCP/UDP port
```

- local access to RTSP is not a problem for [FFmpeg](#source-ffmpeg) integration, because it runs locally on your server
- local access to API is not a problem for the [Home Assistant add-on](#go2rtc-home-assistant-add-on), because Home Assistant runs locally on the same server, and the add-on web UI is protected with Home Assistant authorization ([Ingress feature](https://www.home-assistant.io/blog/2019/04/15/hassio-ingress/))
- external access to WebRTC TCP port is not a problem, because it is used only for transmitting encrypted media data
  - anyway you need to open this port to your local network and to the Internet for WebRTC to work

If you need web interface protection without the Home Assistant add-on, you need to use a reverse proxy, like [Nginx](https://nginx.org/), [Caddy](https://caddyserver.com/), etc.

PS. Additionally, WebRTC will try to use the 8555 UDP port to transmit encrypted media. It works without problems on the local network, and sometimes also works for external access, even if you haven't opened this port on your router ([read more](https://en.wikipedia.org/wiki/UDP_hole_punching)). But for stable external WebRTC access, you need to open the 8555 port on your router for both TCP and UDP.

# Codecs filters

go2rtc can automatically detect which codecs your device supports for [WebRTC](#module-webrtc) and [MSE](#module-mp4) technologies.

But it cannot be done for [RTSP](#module-rtsp), [HTTP progressive streaming](#module-mp4), [HLS](#module-hls) technologies. You can manually add a codec filter when you create a link to a stream. The filters work the same for all three technologies. Filters do not create a new codec. They only select the suitable codec from existing sources. You can add new codecs to the stream using the [FFmpeg transcoding](#source-ffmpeg).

Without filters:

- RTSP will provide only the first video and only the first audio (any codec)
- MP4 will include only compatible codecs (H264, H265, AAC)
- HLS will output in the legacy TS format (H264 without audio)

Some examples:

- `rtsp://192.168.1.123:8554/camera1?mp4` - useful for recording as MP4 files (e.g. Home Assistant or Frigate)
- `rtsp://192.168.1.123:8554/camera1?video=h264,h265&audio=aac` - full version of the filter above
- `rtsp://192.168.1.123:8554/camera1?video=h264&audio=aac&audio=opus` - H264 video codec and two separate audio tracks
- `rtsp://192.168.1.123:8554/camera1?video&audio=all` - any video codec and all audio codecs as separate tracks
- `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4` - HLS stream with MP4 compatible codecs (HLS/fMP4)
- `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4=flac` - HLS stream with PCMA/PCMU/PCM audio support (HLS/fMP4), won't work on old devices
- `http://192.168.1.123:1984/api/stream.mp4?src=camera1&mp4=flac` - MP4 file with PCMA/PCMU/PCM audio support, won't work on old devices (ex. iOS 12)
- `http://192.168.1.123:1984/api/stream.mp4?src=camera1&mp4=all` - MP4 file with non-standard audio codecs, won't work on some players

# Codecs madness

`AVC/H.264` video can be played almost anywhere. But `HEVC/H.265` has many limitations in supporting different devices and browsers.

| Device                                                             | WebRTC                                  | MSE                                     | HTTP*                                        | HLS                         |
|--------------------------------------------------------------------|-----------------------------------------|-----------------------------------------|----------------------------------------------|-----------------------------|
| *latency*                                                          | best                                    | medium                                  | bad                                          | bad                         |
| Desktop Chrome 136+ <br/> Desktop Edge <br/> Android Chrome 136+   | H264, H265* <br/> PCMU, PCMA <br/> OPUS | H264, H265* <br/> AAC, FLAC* <br/> OPUS | H264, H265* <br/> AAC, FLAC* <br/> OPUS, MP3 | no                          |
| Desktop Firefox                                                    | H264 <br/> PCMU, PCMA <br/> OPUS        | H264 <br/> AAC, FLAC* <br/> OPUS        | H264 <br/> AAC, FLAC* <br/> OPUS             | no                          |
| Desktop Safari 14+ <br/> iPad Safari 14+ <br/> iPhone Safari 17.1+ | H264, H265* <br/> PCMU, PCMA <br/> OPUS | H264, H265 <br/> AAC, FLAC*             | **no!**                                      | H264, H265 <br/> AAC, FLAC* |
| iPhone Safari 14+                                                  | H264, H265* <br/> PCMU, PCMA <br/> OPUS | **no!**                                 | **no!**                                      | H264, H265 <br/> AAC, FLAC* |
| macOS [Hass App][1]                                                | no                                      | no                                      | no                                           | H264, H265 <br/> AAC, FLAC* |

[1]: https://apps.apple.com/app/home-assistant/id1099568401

- `HTTP*` - HTTP Progressive Streaming, not related to [progressive download](https://en.wikipedia.org/wiki/Progressive_download), because the file has no size and no end
- `WebRTC H265` - supported in [Chrome 136+](https://developer.chrome.com/release-notes/136), supported in [Safari 18+](https://developer.apple.com/documentation/safari-release-notes/safari-18-release-notes)
- `MSE iPhone` - supported in [iOS 17.1+](https://webkit.org/blog/14735/webkit-features-in-safari-17-1/)

**Audio**

- go2rtc supports [automatic repackaging](#built-in-transcoding) of `PCMA/PCMU/PCM` codecs into `FLAC` for MSE/MP4/HLS so they'll work almost anywhere
- **WebRTC** audio codecs: `PCMU/8000`, `PCMA/8000`, `OPUS/48000/2`
- `OPUS` and `MP3` inside **MP4** are part of the standard, but some players do not support them anyway (especially Apple)

**Apple devices**

- all Apple devices don't support HTTP progressive streaming
- old iPhone firmwares don't support MSE technology because it competes with the HTTP Live Streaming (HLS) technology, invented by Apple
- HLS is the worst technology for **live** streaming, it still exists only because of iPhones

**Codec names**

- H264 = H.264 = AVC (Advanced Video Coding)
- H265 = H.265 = HEVC (High Efficiency Video Coding)
- PCMA = G.711 PCM (A-law) = PCM A-law (`alaw`)
- PCMU = G.711 PCM (µ-law) = PCM mu-law (`mulaw`)
- PCM = L16 = PCM signed 16-bit big-endian (`s16be`)
- AAC = MPEG4-GENERIC
- MP3 = MPEG-1 Audio Layer III or MPEG-2 Audio Layer III

# Built-in transcoding

There are no plans to embed complex transcoding algorithms inside go2rtc. [FFmpeg source](#source-ffmpeg) does a great job with this. Including [hardware acceleration](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration) support.

But go2rtc has some simple algorithms. They are turned on automatically; you do not need to set them up additionally.

**PCM for MSE/MP4/HLS**

Go2rtc can pack `PCMA`, `PCMU` and `PCM` codecs into an MP4 container so that they work in all browsers and all built-in players on modern devices. Including Apple QuickTime:

```text
PCMA/PCMU => PCM => FLAC => MSE/MP4/HLS
```

**Resample PCMA/PCMU for WebRTC**

By default WebRTC supports only `PCMA/8000` and `PCMU/8000`. But go2rtc can automatically resample PCMA and PCMU codecs with a different sample rate. Also, go2rtc can transcode `PCM` codec to `PCMA/8000`, so WebRTC can play it:

```text
PCM/xxx => PCMA/8000 => WebRTC
PCMA/xxx => PCMA/8000 => WebRTC
PCMU/xxx => PCMU/8000 => WebRTC
```

**Important**

- FLAC codec not supported in an RTSP stream. If you are using Frigate or Home Assistant for recording MP4 files with PCMA/PCMU/PCM audio, you should set up transcoding to the AAC codec.
- PCMA and PCMU are VERY low-quality codecs. They support only 256! different sounds. Use them only when you have no other options.

# Codecs negotiation

For example, you want to watch an RTSP stream from a [Dahua IPC-K42](https://www.dahuasecurity.com/fr/products/All-Products/Network-Cameras/Wireless-Series/Wi-Fi-Series/4MP/IPC-K42) camera in your Chrome browser.

- this camera supports two-way audio standard **ONVIF Profile T**
- this camera supports codecs **H264, H265** for sending video, and you select `H264` in camera settings
- this camera supports codecs **AAC, PCMU, PCMA** for sending audio (from mic), and you select `AAC/16000` in camera settings
- this camera supports codecs **AAC, PCMU, PCMA** for receiving audio (to speaker), you don't need to select them
- your browser supports codecs **H264, VP8, VP9, AV1** for receiving video, you don't need to select them
- your browser supports codecs **OPUS, PCMU, PCMA** for sending and receiving audio, you don't need to select them
- you can't get the camera audio directly because its audio codecs don't match your browser's codecs
  - so you decide to use transcoding via FFmpeg and add this setting to the config YAML file
  - you have chosen `OPUS/48000/2` codec, because it is higher quality than the `PCMU/8000` or `PCMA/8000`

Now you have a stream with two sources - **RTSP and FFmpeg**:

```yaml
streams:
  dahua:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - ffmpeg:rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0#audio=opus
```

**go2rtc** automatically matches codecs for your browser across all of your stream sources. This is called **multi-source two-way codec negotiation**, and it's one of the main features of this app.

![Codec negotiation](assets/codecs.svg)

**PS.** You can select `PCMU` or `PCMA` codec in camera settings and not use transcoding at all. Or you can select `AAC` codec for main stream and `PCMU` codec for second stream and add both RTSP to YAML config, this also will work fine.

# Projects using go2rtc

- [Home Assistant](https://www.home-assistant.io/) [2024.11+](https://www.home-assistant.io/integrations/go2rtc/) - top open-source smart home project
- [Frigate](https://frigate.video/) [0.12+](https://docs.frigate.video/guides/configuring_go2rtc/) - open-source NVR built around real-time AI object detection
- [Advanced Camera Card](https://github.com/dermotduffy/advanced-camera-card) - custom card for Home Assistant
- [OpenIPC](https://github.com/OpenIPC/firmware/tree/master/general/package/go2rtc) - alternative IP camera firmware from an open community
- [wz_mini_hacks](https://github.com/gtxaspec/wz_mini_hacks) - custom firmware for Wyze cameras
- [EufyP2PStream](https://github.com/oischinger/eufyp2pstream) - a small project that provides a video/audio stream from Eufy cameras that don't directly support RTSP
- [ioBroker.euSec](https://github.com/bropat/ioBroker.eusec) - [ioBroker](https://www.iobroker.net/) adapter for controlling Eufy security devices
- [MMM-go2rtc](https://github.com/Anonym-tsk/MMM-go2rtc) - MagicMirror² module
- [ring-mqtt](https://github.com/tsightler/ring-mqtt) - Ring-to-MQTT bridge
- [lightNVR](https://github.com/opensensor/lightNVR)

**Distributions**

- [Alpine Linux](https://pkgs.alpinelinux.org/packages?name=go2rtc)
- [Arch User Repository](https://linux-packages.com/aur/package/go2rtc)
- [Gentoo](https://github.com/inode64/inode64-overlay/tree/main/media-video/go2rtc)
- [NixOS](https://search.nixos.org/packages?query=go2rtc)
- [Proxmox Helper Scripts](https://github.com/community-scripts/ProxmoxVE/)
- [QNAP](https://www.myqnap.org/product/go2rtc/)
- [Synology NAS](https://synocommunity.com/package/go2rtc)
- [Unraid](https://unraid.net/community/apps?q=go2rtc)

# Camera experience

- [Dahua](https://www.dahuasecurity.com/) - reference implementation streaming protocols, a lot of settings, high stream quality, multiple streaming clients
- [EZVIZ](https://www.ezviz.com/) - awful RTSP protocol implementation, many bugs in SDP
- [Hikvision](https://www.hikvision.com/) - a lot of proprietary streaming technologies
- [Reolink](https://reolink.com/) - some models have an awful, unusable RTSP implementation and not the best RTMP alternative (I recommend that you contact Reolink support for new firmware), few settings
- [Sonoff](https://sonoff.tech/) - very low stream quality, no settings, not the best protocol implementation
- [TP-Link](https://www.tp-link.com/) - few streaming clients, packet loss?
- Chinese cheap noname cameras, Wyze Cams, Xiaomi cameras with hacks (usually have `/live/ch00_1` in RTSP URL) - awful but usable RTSP protocol implementation, low stream quality, few settings, packet loss?

# Tips

**Using apps for low RTSP delay**

- `ffplay -fflags nobuffer -flags low_delay "rtsp://192.168.1.123:8554/camera1"`
- VLC > Preferences > Input / Codecs > Default Caching Level: Lowest Latency

**Snapshots to Telegram**

[read more](https://github.com/AlexxIT/go2rtc/wiki/Snapshot-to-Telegram)
