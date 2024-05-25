<h1 align="center">

  ![go2rtc](assets/logo.gif)
  <br>
  [![stars](https://img.shields.io/github/stars/AlexxIT/go2rtc?style=flat-square&logo=github)](https://github.com/AlexxIT/go2rtc/stargazers) 
  [![docker pulls](https://img.shields.io/docker/pulls/alexxit/go2rtc?style=flat-square&logo=docker&logoColor=white&label=pulls)](https://hub.docker.com/r/alexxit/go2rtc) 
  [![releases](https://img.shields.io/github/downloads/AlexxIT/go2rtc/total?color=blue&style=flat-square&logo=github)](https://github.com/AlexxIT/go2rtc/releases)
  [![goreport](https://goreportcard.com/badge/github.com/AlexxIT/go2rtc)](https://goreportcard.com/report/github.com/AlexxIT/go2rtc)
</h1>

Ultimate camera streaming application with support RTSP, WebRTC, HomeKit, FFmpeg, RTMP, etc.

![](assets/go2rtc.png)

- zero-dependency and zero-config [small app](#go2rtc-binary) for all OS (Windows, macOS, Linux, ARM)
- zero-delay for many supported protocols (lowest possible streaming latency)
- streaming from [RTSP](#source-rtsp), [RTMP](#source-rtmp), [DVRIP](#source-dvrip), [HTTP](#source-http) (FLV/MJPEG/JPEG/TS), [USB Cameras](#source-ffmpeg-device) and [other sources](#module-streams)
- streaming from any sources, supported by [FFmpeg](#source-ffmpeg)
- streaming to [RTSP](#module-rtsp), [WebRTC](#module-webrtc), [MSE/MP4](#module-mp4), [HomeKit](#module-homekit) [HLS](#module-hls) or [MJPEG](#module-mjpeg)
- [publish](#publish-stream) any source to popular streaming services (YouTube, Telegram, etc.)
- first project in the World with support streaming from [HomeKit Cameras](#source-homekit)
- support H265 for WebRTC in browser (Safari only, [read more](https://github.com/AlexxIT/Blog/issues/5))
- on the fly transcoding for unsupported codecs via [FFmpeg](#source-ffmpeg)
- play audio files and live streams on some cameras with [speaker](#stream-to-camera)
- multi-source 2-way [codecs negotiation](#codecs-negotiation)
   - mixing tracks from different sources to single stream
   - auto match client supported codecs
   - [2-way audio](#two-way-audio) for some cameras
- streaming from private networks via [ngrok](#module-ngrok)
- can be [integrated to](#module-api) any smart home platform or be used as [standalone app](#go2rtc-binary)

**Inspired by:**

- series of streaming projects from [@deepch](https://github.com/deepch)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [rtsp-simple-server](https://github.com/aler9/rtsp-simple-server) idea from [@aler9](https://github.com/aler9)
- [GStreamer](https://gstreamer.freedesktop.org/) framework pipeline idea
- [MediaSoup](https://mediasoup.org/) framework routing idea
- HomeKit Accessory Protocol from [@brutella](https://github.com/brutella/hap)
- creator of the project's logo [@v_novoseltsev](https://www.instagram.com/v_novoseltsev) 

---

* [Fast start](#fast-start)
  * [go2rtc: Binary](#go2rtc-binary)
  * [go2rtc: Docker](#go2rtc-docker)
  * [go2rtc: Home Assistant Add-on](#go2rtc-home-assistant-add-on)
  * [go2rtc: Home Assistant Integration](#go2rtc-home-assistant-integration)
  * [go2rtc: Dev version](#go2rtc-dev-version)
* [Configuration](#configuration)
  * [Module: Streams](#module-streams)
    * [Two way audio](#two-way-audio)
    * [Source: RTSP](#source-rtsp)
    * [Source: RTMP](#source-rtmp)
    * [Source: HTTP](#source-http)
    * [Source: ONVIF](#source-onvif)
    * [Source: FFmpeg](#source-ffmpeg)
    * [Source: FFmpeg Device](#source-ffmpeg-device)
    * [Source: Exec](#source-exec)
    * [Source: Echo](#source-echo)
    * [Source: Expr](#source-expr)
    * [Source: HomeKit](#source-homekit)
    * [Source: Bubble](#source-bubble)
    * [Source: DVRIP](#source-dvrip)
    * [Source: Tapo](#source-tapo)
    * [Source: Kasa](#source-kasa)
    * [Source: GoPro](#source-gopro)
    * [Source: Ivideon](#source-ivideon)
    * [Source: Hass](#source-hass)
    * [Source: ISAPI](#source-isapi)
    * [Source: Nest](#source-nest)
    * [Source: Roborock](#source-roborock)
    * [Source: WebRTC](#source-webrtc)
    * [Source: WebTorrent](#source-webtorrent)
    * [Incoming sources](#incoming-sources)
    * [Stream to camera](#stream-to-camera)
    * [Publish stream](#publish-stream)
  * [Module: API](#module-api)
  * [Module: RTSP](#module-rtsp)
  * [Module: RTMP](#module-rtmp)
  * [Module: WebRTC](#module-webrtc)
  * [Module: HomeKit](#module-homekit)
  * [Module: WebTorrent](#module-webtorrent)
  * [Module: ngrok](#module-ngrok)
  * [Module: Hass](#module-hass)
  * [Module: MP4](#module-mp4)
  * [Module: HLS](#module-hls)
  * [Module: MJPEG](#module-mjpeg)
  * [Module: Log](#module-log)
* [Security](#security)
* [Codecs filters](#codecs-filters)
* [Codecs madness](#codecs-madness)
* [Codecs negotiation](#codecs-negotiation)
* [Projects using go2rtc](#projects-using-go2rtc)
* [Camera experience](#cameras-experience)
* [TIPS](#tips)
* [FAQ](#faq)

## Fast start

1. Download [binary](#go2rtc-binary) or use [Docker](#go2rtc-docker) or Home Assistant [Add-on](#go2rtc-home-assistant-add-on) or [Integration](#go2rtc-home-assistant-integration)
2. Open web interface: `http://localhost:1984/`

**Optionally:**

- add your [streams](#module-streams) to [config](#configuration) file
- setup [external access](#module-webrtc) to webrtc

**Developers:**

- write your own [web interface](#module-api)
- integrate [web api](#module-api) into your smart home platform

### go2rtc: Binary

Download binary for your OS from [latest release](https://github.com/AlexxIT/go2rtc/releases/):

- `go2rtc_win64.zip` - Windows 64-bit
- `go2rtc_win32.zip` - Windows 32-bit
- `go2rtc_win_arm64.zip` - Windows ARM 64-bit
- `go2rtc_linux_amd64` - Linux 64-bit
- `go2rtc_linux_i386` - Linux 32-bit
- `go2rtc_linux_arm64` - Linux ARM 64-bit (ex. Raspberry 64-bit OS)
- `go2rtc_linux_arm` - Linux ARM 32-bit (ex. Raspberry 32-bit OS)
- `go2rtc_linux_armv6` - Linux ARMv6 (for old Raspberry 1 and Zero)
- `go2rtc_linux_mipsel` - Linux MIPS (ex. [Xiaomi Gateway 3](https://github.com/AlexxIT/XiaomiGateway3), [Wyze cameras](https://github.com/gtxaspec/wz_mini_hacks))
- `go2rtc_mac_amd64.zip` - Mac Intel 64-bit
- `go2rtc_mac_arm64.zip` - Mac ARM 64-bit

Don't forget to fix the rights `chmod +x go2rtc_xxx_xxx` on Linux and Mac.

### go2rtc: Docker

The Docker container [`alexxit/go2rtc`](https://hub.docker.com/r/alexxit/go2rtc) supports multiple architectures including `amd64`, `386`, `arm64`, and `arm`. This container offers the same functionality as the [Home Assistant Add-on](#go2rtc-home-assistant-add-on) but is designed to operate independently of Home Assistant. It comes preinstalled with [FFmpeg](#source-ffmpeg), [ngrok](#module-ngrok), and [Python](#source-echo).

### go2rtc: Home Assistant Add-on

[![](https://my.home-assistant.io/badges/supervisor_addon.svg)](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons)

1. Install Add-On:
    - Settings > Add-ons > Plus > Repositories > Add `https://github.com/AlexxIT/hassio-addons`
    - go2rtc > Install > Start
2. Setup [Integration](#module-hass)

### go2rtc: Home Assistant Integration

[WebRTC Camera](https://github.com/AlexxIT/WebRTC) custom component can be used on any [Home Assistant installation](https://www.home-assistant.io/installation/), including [HassWP](https://github.com/AlexxIT/HassWP) on Windows. It can automatically download and use the latest version of go2rtc. Or it can connect to an existing version of go2rtc. Addon installation in this case is optional.

### go2rtc: Dev version

Latest, but maybe unstable version:

- Binary: [latest nightly release](https://nightly.link/AlexxIT/go2rtc/workflows/build/master)
- Docker: `alexxit/go2rtc:master` or `alexxit/go2rtc:master-hardware` versions
- Hass Add-on: `go2rtc master` or `go2rtc master hardware` versions

## Configuration

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
- [mp4](#module-mp4) - MSE, MP4 stream and MP4 shapshot Server
- [hls](#module-hls) - HLS TS or fMP4 stream Server
- [mjpeg](#module-mjpeg) - MJPEG Server
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [ngrok](#module-ngrok) - ngrok integration (external access for private network)
- [hass](#module-hass) - Home Assistant integration
- [log](#module-log) - logs config

### Module: Streams

**go2rtc** support different stream source types. You can config one or multiple links of any type as stream source.

Available source types:

- [rtsp](#source-rtsp) - `RTSP` and `RTSPS` cameras with [two way audio](#two-way-audio) support
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
- [tapo](#source-tapo) - TP-Link Tapo cameras with [two way audio](#two-way-audio) support
- [kasa](#source-tapo) - TP-Link Kasa cameras
- [gopro](#source-gopro) - GoPro cameras
- [ivideon](#source-ivideon) - public cameras from [Ivideon](https://tv.ivideon.com/) service
- [hass](#source-hass) - Home Assistant integration
- [isapi](#source-isapi) - two way audio for Hikvision (ISAPI) cameras
- [roborock](#source-roborock) - Roborock vacuums with cameras
- [webrtc](#source-webrtc) - WebRTC/WHEP sources
- [webtorrent](#source-webtorrent) - WebTorrent source from another go2rtc

Read more about [incoming sources](#incoming-sources)

#### Two way audio

Supported for sources:

- [RTSP cameras](#source-rtsp) with [ONVIF Profile T](https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf) (back channel connection)
- [DVRIP](#source-dvrip) cameras
- [TP-Link Tapo](#source-tapo) cameras
- [Hikvision ISAPI](#source-isapi) cameras
- [Roborock vacuums](#source-roborock) models with cameras
- [Exec](#source-exec) audio on server
- [Any Browser](#incoming-browser) as IP-camera

Two way audio can be used in browser with [WebRTC](#module-webrtc) technology. The browser will give access to the microphone only for HTTPS sites ([read more](https://stackoverflow.com/questions/52759992/how-to-access-camera-and-microphone-in-chrome-without-https)).

go2rtc also support [play audio](#stream-to-camera) files and live streams on this cameras.

#### Source: RTSP

```yaml
streams:
  sonoff_camera: rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
  dahua_camera:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=1
  amcrest_doorbell:
    - rtsp://username:password@192.168.1.123:554/cam/realmonitor?channel=1&subtype=0#backchannel=0
  unifi_camera: rtspx://192.168.1.123:7441/fD6ouM72bWoFijxK
  glichy_camera: ffmpeg:rtsp://username:password@192.168.1.123/live/ch00_1 
```

**Recommendations**

- **Amcrest Doorbell** users may want to disable two way audio, because with an active stream you won't have a call button working. You need to add `#backchannel=0` to the end of your RTSP link in YAML config file
- **Dahua Doorbell** users may want to change backchannel [audio codec](https://github.com/AlexxIT/go2rtc/issues/52)
- **Reolink** users may want NOT to use RTSP protocol at all, some camera models have a very awful unusable stream implementation
- **Ubiquiti UniFi** users may want to disable HTTPS verification. Use `rtspx://` prefix instead of `rtsps://`. And don't use `?enableSrtp` [suffix](https://github.com/AlexxIT/go2rtc/issues/81)
- **TP-Link Tapo** users may skip login and password, because go2rtc support login [without them](https://drmnsamoliu.github.io/video.html)
- If your camera has two RTSP links - you can add both of them as sources. This is useful when streams has different codecs, as example AAC audio with main stream and PCMU/PCMA audio with second stream
- If the stream from your camera is glitchy, try using [ffmpeg source](#source-ffmpeg). It will not add CPU load if you won't use transcoding
- If the stream from your camera is very glitchy, try to use transcoding with [ffmpeg source](#source-ffmpeg)

**Other options**

Format: `rtsp...#{param1}#{param2}#{param3}`

- Add custom timeout `#timeout=30` (in seconds)
- Ignore audio - `#media=video` or ignore video - `#media=audio` 
- Ignore two way audio API `#backchannel=0` - important for some glitchy cameras
- Use WebSocket transport `#transport=ws...`

**RTSP over WebSocket**

```yaml
streams:
  # WebSocket with authorization, RTSP - without
  axis-rtsp-ws:  rtsp://192.168.1.123:4567/axis-media/media.amp?overview=0&camera=1&resolution=1280x720&videoframeskipmode=empty&Axis-Orig-Sw=true#transport=ws://user:pass@192.168.1.123:4567/rtsp-over-websocket
  # WebSocket without authorization, RTSP - with
  dahua-rtsp-ws: rtsp://user:pass@192.168.1.123/cam/realmonitor?channel=1&subtype=1&proto=Private3#transport=ws://192.168.1.123/rtspoverwebsocket
```

#### Source: RTMP

You can get stream from RTMP server, for example [Nginx with nginx-rtmp-module](https://github.com/arut/nginx-rtmp-module).

```yaml
streams:
  rtmp_stream: rtmp://192.168.1.123/live/camera1
```

#### Source: HTTP

Support Content-Type:

- **HTTP-FLV** (`video/x-flv`) - same as RTMP, but over HTTP
- **HTTP-JPEG** (`image/jpeg`) - camera snapshot link, can be converted by go2rtc to MJPEG stream
- **HTTP-MJPEG** (`multipart/x`) - simple MJPEG stream over HTTP
- **MPEG-TS** (`video/mpeg`) - legacy [streaming format](https://en.wikipedia.org/wiki/MPEG_transport_stream)

Source also support HTTP and TCP streams with autodetection for different formats: **MJPEG**, **H.264/H.265 bitstream**, **MPEG-TS**.

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

**PS.** Dahua camera has bug: if you select MJPEG codec for RTSP second stream - snapshot won't work.

#### Source: ONVIF

*[New in v1.5.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)*

The source is not very useful if you already know RTSP and snapshot links for your camera. But it can be useful if you don't.

**WebUI > Add** webpage support ONVIF autodiscovery. Your server must be on the same subnet as the camera. If you use docker, you must use "network host".

```yaml
streams:
  dahua1: onvif://admin:password@192.168.1.123
  reolink1: onvif://admin:password@192.168.1.123:8000
  tapo1: onvif://admin:password@192.168.1.123:2020
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
  mjpeg: ffmpeg:http://185.97.122.128/cgi-bin/faststream.jpg#video=h264

  # [RTSP] video with rotation, should be transcoded, so select H264
  rotate: ffmpeg:rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0#video=h264#rotate=90
```

All trascoding formats has [built-in templates](https://github.com/AlexxIT/go2rtc/blob/master/internal/ffmpeg/ffmpeg.go): `h264`, `h265`, `opus`, `pcmu`, `pcmu/16000`, `pcmu/48000`, `pcma`, `pcma/16000`, `pcma/48000`, `aac`, `aac/16000`.

But you can override them via YAML config. You can also add your own formats to config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg  # path to ffmpeg binary
  h264: "-codec:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency -profile:v main -level:v 4.1"
  mycodec: "-any args that supported by ffmpeg..."
  myinput: "-fflags nobuffer -flags low_delay -timeout 5000000 -i {input}"
  myraw: "-ss 00:00:20"
```

- You can use go2rtc stream name as ffmpeg input (ex. `ffmpeg:camera1#video=h264`)
- You can use `video` and `audio` params multiple times (ex. `#video=copy#audio=copy#audio=pcmu`)
- You can use `rotate` param with `90`, `180`, `270` or `-90` values, important with transcoding (ex. `#video=h264#rotate=90`)
- You can use `width` and/or `height` params, important with transcoding (ex. `#video=h264#width=1280`)
- You can use `drawtext` to add a timestamp (ex. `drawtext=x=2:y=2:fontsize=12:fontcolor=white:box=1:boxcolor=black`)
  - This will greatly increase the CPU of the server, even with hardware acceleration
- You can use `raw` param for any additional FFmpeg arguments (ex. `#raw=-vf transpose=1`)
- You can use `input` param to override default input template (ex. `#input=rtsp/udp` will change RTSP transport from TCP to UDP+TCP)
  - You can use raw input value (ex. `#input=-timeout 5000000 -i {input}`)
  - You can add your own input templates

Read more about [hardware acceleration](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration).

**PS.** It is recommended to check the available hardware in the WebUI add page.

#### Source: FFmpeg Device

You can get video from any USB-camera or Webcam as RTSP or WebRTC stream. This is part of FFmpeg integration.

- check available devices in Web interface
- `video_size` and `framerate` must be supported by your camera!
- for Linux supported only video for now
- for macOS you can stream Facetime camera or whole Desktop!
- for macOS important to set right framerate

Format: `ffmpeg:device?{input-params}#{param1}#{param2}#{param3}`

```yaml
streams:
  linux_usbcam:   ffmpeg:device?video=0&video_size=1280x720#video=h264
  windows_webcam: ffmpeg:device?video=0#video=h264
  macos_facetime: ffmpeg:device?video=0&audio=1&video_size=1280x720&framerate=30#video=h264#audio=pcma
```

**PS.** It is recommended to check the available devices in the WebUI add page.

#### Source: Exec

Exec source can run any external application and expect data from it. Two transports are supported - **pipe** (*from [v1.5.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)*) and **RTSP**.

If you want to use **RTSP** transport - the command must contain the `{output}` argument in any place. On launch, it will be replaced by the local address of the RTSP server.

**pipe** reads data from app stdout in different formats: **MJPEG**, **H.264/H.265 bitstream**, **MPEG-TS**. Also pipe can write data to app stdin in two formats: **PCMA** and **PCM/48000**.

The source can be used with:

- [FFmpeg](https://ffmpeg.org/) - go2rtc ffmpeg source just a shortcut to exec source
- [FFplay](https://ffmpeg.org/ffplay.html) - play audio on your server
- [GStreamer](https://gstreamer.freedesktop.org/)
- [Raspberry Pi Cameras](https://www.raspberrypi.com/documentation/computers/camera_software.html)
- any your own software

Pipe commands support parameters (format: `exec:{command}#{param1}#{param2}`):

- `killsignal` - signal which will be send to stop the process (numeric form)
- `killtimeout` - time in seconds for forced termination with sigkill
- `backchannel` - enable backchannel for two-way audio

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

#### Source: Echo

Some sources may have a dynamic link. And you will need to get it using a bash or python script. Your script should echo a link to the source. RTSP, FFmpeg or any of the [supported sources](#module-streams).

**Docker** and **Hass Add-on** users has preinstalled `python3`, `curl`, `jq`.

Check examples in [wiki](https://github.com/AlexxIT/go2rtc/wiki/Source-Echo-examples).

```yaml
streams:
  apple_hls: echo:python3 hls.py https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html
```

#### Source: Expr

*[New in v1.8.2](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.2)*

Like `echo` source, but uses the built-in [expr](https://github.com/antonmedv/expr) expression language ([read more](https://github.com/AlexxIT/go2rtc/blob/master/internal/expr/README.md)).

#### Source: HomeKit

**Important:**

- You can use HomeKit Cameras **without Apple devices** (iPhone, iPad, etc.), it's just a yet another protocol
- HomeKit device can be paired with only one ecosystem. So, if you have paired it to an iPhone (Apple Home) - you can't pair it with Home Assistant or go2rtc. Or if you have paired it to go2rtc - you can't pair it with iPhone
- HomeKit device should be in same network with working [mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) between device and go2rtc

go2rtc support import paired HomeKit devices from [Home Assistant](#source-hass). So you can use HomeKit camera with Hass and go2rtc simultaneously. If you using Hass, I recommend pairing devices with it, it will give you more options.

You can pair device with go2rtc on the HomeKit page. If you can't see your devices - reload the page. Also try reboot your HomeKit device (power off). If you still can't see it - you have a problems with mDNS.

If you see a device but it does not have a pair button - it is paired to some ecosystem (Apple Home, Home Assistant, HomeBridge etc). You need to delete device from that ecosystem, and it will be available for pairing. If you cannot unpair device, you will have to reset it.

**Important:**

- HomeKit audio uses very non-standard **AAC-ELD** codec with very non-standard params and specification violation
- Audio can't be played in `VLC` and probably any other player
- Audio should be transcoded for using with MSE, WebRTC, etc.

Recommended settings for using HomeKit Camera with WebRTC, MSE, MP4, RTSP:

```
streams:
  aqara_g3:
    - hass:Camera-Hub-G3-AB12
    - ffmpeg:aqara_g3#audio=aac#audio=opus
```

RTSP link with "normal" audio for any player: `rtsp://192.168.1.123:8554/aqara_g3?video&audio=aac`

**This source is in active development!** Tested only with [Aqara Camera Hub G3](https://www.aqara.com/eu/product/camera-hub-g3) (both EU and CN versions).

#### Source: Bubble

*[New in v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*

Other names: [ESeeCloud](http://www.eseecloud.com/), [dvr163](http://help.dvr163.com/).

- you can skip `username`, `password`, `port`, `ch` and `stream` if they are default
- setup separate streams for different channels and streams

```yaml
streams:
  camera1: bubble://username:password@192.168.1.123:34567/bubble/live?ch=0&stream=0
```

#### Source: DVRIP

*[New in v1.2.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.2.0)*

Other names: DVR-IP, NetSurveillance, Sofia protocol (NETsurveillance ActiveX plugin XMeye SDK).

- you can skip `username`, `password`, `port`, `channel` and `subtype` if they are default
- setup separate streams for different channels
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

#### Source: Tapo

*[New in v1.2.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.2.0)*

[TP-Link Tapo](https://www.tapo.com/) proprietary camera protocol with **two way audio** support.

- stream quality is the same as [RTSP protocol](https://www.tapo.com/en/faq/34/)
- use the **cloud password**, this is not the RTSP password! you do not need to add a login!
- you can also use UPPERCASE MD5 hash from your cloud password with `admin` username
- some new camera firmwares requires SHA256 instead of MD5

```yaml
streams:
  # cloud password without username
  camera1: tapo://cloud-password@192.168.1.123
  # admin username and UPPERCASE MD5 cloud-password hash
  camera2: tapo://admin:UPPERCASE-MD5@192.168.1.123
  # admin username and UPPERCASE SHA256 cloud-password hash
  camera3: tapo://admin:UPPERCASE-SHA256@192.168.1.123
```

```bash
echo -n "cloud password" | md5 | awk '{print toupper($0)}'
echo -n "cloud password" | shasum -a 256 | awk '{print toupper($0)}'
```

#### Source: Kasa

*[New in v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*

[TP-Link Kasa](https://www.kasasmart.com/) non-standard protocol [more info](https://medium.com/@hu3vjeen/reverse-engineering-tp-link-kc100-bac4641bf1cd).

- `username` - urlsafe email, `alex@gmail.com` -> `alex%40gmail.com`
- `password` - base64password, `secret1` -> `c2VjcmV0MQ==`

```yaml
streams:
  kc401: kasa://username:password@192.168.1.123:19443/https/stream/mixed
```

Tested: KD110, KC200, KC401, KC420WS, EC71.

#### Source: GoPro

*[New in v1.8.3](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.3)*

Support streaming from [GoPro](https://gopro.com/) cameras, connected via USB or Wi-Fi to Linux, Mac, Windows. [Read more](https://github.com/AlexxIT/go2rtc/tree/master/internal/gopro).

#### Source: Ivideon

Support public cameras from service [Ivideon](https://tv.ivideon.com/).

```yaml
streams:
  quailcam: ivideon:100-tu5dkUPct39cTp9oNEN2B6/0
```

#### Source: Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI
- [HomeKit Camera](https://www.home-assistant.io/integrations/homekit_controller/)
- [ONVIF](https://www.home-assistant.io/integrations/onvif/)
- [Roborock](https://github.com/humbertogontijo/homeassistant-roborock) vacuums with camera

```yaml
hass:
  config: "/config"  # skip this setting if you Hass Add-on user

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
  aqara_g3: hass:Camera-Hub-G3-AB12
```

**WebRTC Cameras** (*from [v1.6.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.0)*)

Any cameras in WebRTC format are supported. But at the moment Home Assistant only supports some [Nest](https://www.home-assistant.io/integrations/nest/) cameras in this fomat.

**Important.** The Nest API only allows you to get a link to a stream for 5 minutes. Do not use this with Frigate! If the stream expires, Frigate will consume all available ram on your machine within seconds. It's recommended to use [Nest source](#source-nest) - it supports extending the stream.

```yaml
streams:
  # link to Home Assistant Supervised
  hass-webrtc1: hass://supervisor?entity_id=camera.nest_doorbell
  # link to external Hass with Long-Lived Access Tokens
  hass-webrtc2: hass://192.168.1.123:8123?entity_id=camera.nest_doorbell&token=eyXYZ...
```

**RTSP Cameras**

By default, the Home Assistant API does not allow you to get dynamic RTSP link to a camera stream. So more cameras, like [Tuya](https://www.home-assistant.io/integrations/tuya/), and possibly others can also be imported by using [this method](https://github.com/felipecrs/hass-expose-camera-stream-source#importing-home-assistant-cameras-to-go2rtc-andor-frigate).

#### Source: ISAPI

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type support only backchannel audio for Hikvision ISAPI protocol. So it should be used as second source in addition to the RTSP protocol.

```yaml
streams:
  hikvision1:
    - rtsp://admin:password@192.168.1.123:554/Streaming/Channels/101
    - isapi://admin:password@192.168.1.123:80/
```

#### Source: Nest

*[New in v1.6.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.0)*

Currently only WebRTC cameras are supported.

For simplicity, it is recommended to connect the Nest/WebRTC camera to the [Home Assistant](#source-hass). But if you can somehow get the below parameters - Nest/WebRTC source will work without Hass.

```yaml
streams:
  nest-doorbell: nest:?client_id=***&client_secret=***&refresh_token=***&project_id=***&device_id=***
```

#### Source: Roborock

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type support Roborock vacuums with cameras. Known working models:

- Roborock S6 MaxV - only video (the vacuum has no microphone)
- Roborock S7 MaxV - video and two way audio

Source support load Roborock credentials from Home Assistant [custom integration](https://github.com/humbertogontijo/homeassistant-roborock). Otherwise, you need to log in to your Roborock account (MiHome account is not supported). Go to: go2rtc WebUI > Add webpage. Copy `roborock://...` source for your vacuum and paste it to `go2rtc.yaml` config.

If you have graphic pin for your vacuum - add it as numeric pin (lines: 123, 456, 678) to the end of the roborock-link.

#### Source: WebRTC

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source type support four connection formats.

**whep**

[WebRTC/WHEP](https://datatracker.ietf.org/doc/draft-murillo-whep/) - is replaced by [WebRTC/WISH](https://datatracker.ietf.org/doc/charter-ietf-wish/02/) standard for WebRTC video/audio viewers. But it may already be supported in some third-party software. It is supported in go2rtc.

**go2rtc**

This format is only supported in go2rtc. Unlike WHEP it supports asynchronous WebRTC connection and two way audio.

**openipc** (*from [v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*)

Support connection to [OpenIPC](https://openipc.org/) cameras.

**wyze** (*from [v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*)

Supports connection to [Wyze](https://www.wyze.com/) cameras, using WebRTC protocol. You can use [docker-wyze-bridge](https://github.com/mrlt8/docker-wyze-bridge) project to get connection credentials.

**kinesis** (*from [v1.6.1](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)*)

Supports [Amazon Kinesis Video Streams](https://aws.amazon.com/kinesis/video-streams/), using WebRTC protocol. You need to specify signalling WebSocket URL with all credentials in query params, `client_id` and `ice_servers` list in [JSON format](https://developer.mozilla.org/en-US/docs/Web/API/RTCIceServer).

```yaml
streams:
  webrtc-whep:    webrtc:http://192.168.1.123:1984/api/webrtc?src=camera1
  webrtc-go2rtc:  webrtc:ws://192.168.1.123:1984/api/ws?src=camera1
  webrtc-openipc: webrtc:ws://192.168.1.123/webrtc_ws#format=openipc#ice_servers=[{"urls":"stun:stun.kinesisvideo.eu-north-1.amazonaws.com:443"}]
  webrtc-wyze:    webrtc:http://192.168.1.123:5000/signaling/camera1?kvs#format=wyze
  webrtc-kinesis: webrtc:wss://...amazonaws.com/?...#format=kinesis#client_id=...#ice_servers=[{...},{...}]
```

**PS.** For `kinesis` sources you can use [echo](#source-echo) to get connection params using `bash`/`python` or any other script language.

#### Source: WebTorrent

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This source can get a stream from another go2rtc via [WebTorrent](#module-webtorrent) protocol.

```yaml
streams:
  webtorrent1: webtorrent:?share=huofssuxaty00izc&pwd=k3l2j9djeg8v8r7e
```

#### Incoming sources

By default, go2rtc establishes a connection to the source when any client requests it. Go2rtc drops the connection to the source when it has no clients left.

- Go2rtc also can accepts incoming sources in [RTSP](#module-rtsp), [RTMP](#module-rtmp), [HTTP](#source-http) and **WebRTC/WHIP** formats
- Go2rtc won't stop such a source if it has no clients
- You can push data only to existing stream (create stream with empty source in config)
- You can push multiple incoming sources to same stream
- You can push data to non empty stream, so it will have additional codecs inside

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

#### Incoming: Browser

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

You can turn the browser of any PC or mobile into an IP-camera with support video and two way audio. Or even broadcast your PC screen:

1. Create empty stream in the `go2rtc.yaml`
2. Go to go2rtc WebUI
3. Open `links` page for you stream
4. Select `camera+microphone` or `display+speaker` option
5. Open `webrtc` local page (your go2rtc **should work over HTTPS!**) or `share link` via [WebTorrent](#module-webtorrent) technology (work over HTTPS by default)

#### Incoming: WebRTC/WHIP

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

You can use **OBS Studio** or any other broadcast software with [WHIP](https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html) protocol support. This standard has not yet been approved. But you can download OBS Studio [dev version](https://github.com/obsproject/obs-studio/actions/runs/3969201209):

- Settings > Stream > Service: WHIP > http://192.168.1.123:1984/api/webrtc?dst=camera1

#### Stream to camera

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

go2rtc support play audio files (ex. music or [TTS](https://www.home-assistant.io/integrations/#text-to-speech)) and live streams (ex. radio) on cameras with [two way audio](#two-way-audio) support (RTSP/ONVIF cameras, TP-Link Tapo, Hikvision ISAPI, Roborock vacuums, any Browser).

API example:

```
POST http://localhost:1984/api/streams?dst=camera1&src=ffmpeg:http://example.com/song.mp3#audio=pcma#input=file
```

- you can stream: local files, web files, live streams or any format, supported by FFmpeg 
- you should use [ffmpeg source](#source-ffmpeg) for transcoding audio to codec, that your camera supports
- you can check camera codecs on the go2rtc WebUI info page when the stream is active
- some cameras support only low quality `PCMA/8000` codec (ex. [Tapo](#source-tapo))
- it is recommended to choose higher quality formats if your camera supports them (ex. `PCMA/48000` for some Dahua cameras)
- if you play files over http-link, you need to add `#input=file` params for transcoding, so file will be transcoded and played in real time
- if you play live streams, you should skip `#input` param, because it is already in real time
- you can stop active playback by calling the API with the empty `src` parameter
- you will see one active producer and one active consumer in go2rtc WebUI info page during streaming

### Publish stream

*[New in v1.8.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)*

You can publish any stream to streaming services (YouTube, Telegram, etc.) via RTMP/RTMPS. Important:

- Supported codecs: H264 for video and AAC for audio
- AAC audio is required for YouTube, videos without audio will not work
- You don't need to enable [RTMP module](#module-rtmp) listening for this task

You can use API:

```
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

### Module: API

The HTTP API is the main part for interacting with the application. Default address: `http://localhost:1984/`.

**Important!** go2rtc passes requests from localhost and from unix socket without HTTP authorisation, even if you have it configured! It is your responsibility to set up secure external access to API. If not properly configured, an attacker can gain access to your cameras and even your server.

[API description](https://github.com/AlexxIT/go2rtc/tree/master/api).

**Module config**

- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "127.0.0.1:1984"` setting
- you can change API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`
- you can use raw TLS cert/key content or path to files

```yaml
api:
  listen: ":1984"    # default ":1984", HTTP API port ("" - disabled)
  username: "admin"  # default "", Basic auth for WebUI
  password: "pass"   # default "", Basic auth for WebUI
  base_path: "/rtc"  # default "", API prefix for serve on suburl (/api => /rtc/api)
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

### Module: RTSP

You can get any stream as RTSP-stream: `rtsp://192.168.1.123:8554/{stream_name}`

You can enable external password protection for your RTSP streams. Password protection always disabled for localhost calls (ex. FFmpeg or Hass on same server).

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

### Module: RTMP

*[New in v1.8.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)*

You can get any stream as RTMP-stream: `rtmp://192.168.1.123/{stream_name}`. Only H264/AAC codecs supported right now.

[Incoming stream](#incoming-sources) in RTMP-format tested only with [OBS Studio](https://obsproject.com/) and Dahua camera. Different FFmpeg versions has differnt problems with this format. 

```yaml
rtmp:
  listen: ":1935"  # by default - disabled!
```

### Module: WebRTC

In most cases [WebRTC](https://en.wikipedia.org/wiki/WebRTC) uses direct peer-to-peer connection from your browser to go2rtc and sends media data via UDP.
It **can't pass** media data through your Nginx or Cloudflare or [Nabu Casa](https://www.nabucasa.com/) HTTP TCP connection!
It can automatically detects your external IP via public [STUN](https://en.wikipedia.org/wiki/STUN) server.
It can establish a external direct connection via [UDP hole punching](https://en.wikipedia.org/wiki/UDP_hole_punching) technology even if you not open your server to the World.

But about 10-20% of users may need to configure additional settings for external access if **mobile phone** or **go2rtc server** behing [Symmetric NAT](https://tomchen.github.io/symmetric-nat-test/).

- by default, WebRTC uses both TCP and UDP on port 8555 for connections
- you can use this port for external access
- you can change the port in YAML config:

```yaml
webrtc:
  listen: ":8555"  # address of your local server and port (TCP/UDP)
```

**Static public IP**

- forward the port 8555 on your router (you can use same 8555 port or any other as external port)
- add your external IP-address and external port to YAML config

```yaml
webrtc:
  candidates:
    - 216.58.210.174:8555  # if you have static public IP-address
```

**Dynamic public IP**

- forward the port 8555 on your router (you can use same 8555 port or any other as the external port)
- add `stun` word and external port to YAML config
  - go2rtc automatically detects your external address with STUN-server

```yaml
webrtc:
  candidates:
    - stun:8555  # if you have dynamic public IP-address
```

**Private IP**

- setup integration with [ngrok service](#module-ngrok)

```yaml
ngrok:
  command: ...
```

**Hard tech way 1. Own TCP-tunnel**

If you have personal [VPS](https://en.wikipedia.org/wiki/Virtual_private_server), you can create TCP-tunnel and setup in the same way as "Static public IP". But use your VPS IP-address in YAML config.

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

### Module: HomeKit

*[New in v1.7.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)*

HomeKit module can work in two modes:

- export any H264 camera to Apple HomeKit
- transparent proxy any Apple HomeKit camera (Aqara, Eve, Eufy, etc.) back to Apple HomeKit, so you will have all camera features in Apple Home and also will have RTSP/WebRTC/MP4/etc. from your HomeKit camera

**Important**

- HomeKit cameras supports only H264 video and OPUS audio

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

### Module: WebTorrent

*[New in v1.3.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)*

This module support:

- Share any local stream via [WebTorrent](https://webtorrent.io/) technology
- Get any [incoming stream](#incoming-browser) from PC or mobile via [WebTorrent](https://webtorrent.io/) technology
- Get any remote [go2rtc source](#source-webtorrent) via [WebTorrent](https://webtorrent.io/) technology

Securely and free. You do not need to open a public access to the go2rtc server. But in some cases (Symmetric NAT) you may need to set up external access to [WebRTC module](#module-webrtc).

To generate sharing link or incoming link - goto go2rtc WebUI (stream links page). This link is **temporary** and will stop working after go2rtc is restarted!

You can create permanent external links in go2rtc config:

```yaml
webtorrent:
  shares:
    super-secret-share:  # share name, should be unique among all go2rtc users!
      pwd: super-secret-password
      src: rtsp-dahua1   # stream name from streams section
```

Link example: https://alexxit.github.io/go2rtc/#share=02SNtgjKXY&pwd=wznEQqznxW&media=video+audio

TODO: article how it works...

### Module: ngrok

With ngrok integration you can get external access to your streams in situations when you have Internet with private IP-address.

- ngrok is pre-installed for **Docker** and **Hass Add-on** users
- you may need external access for two different things:
  - WebRTC stream, so you need tunnel WebRTC TCP port (ex. 8555)
  - go2rtc web interface, so you need tunnel API HTTP port (ex. 1984)
- ngrok support authorization for your web interface
- ngrok automatically adds HTTPS to your web interface

The ngrok free subscription has the following limitations:

- You can reserve a free domain for serving the web interface, but the TCP address you get will always be random and change with each restart of the ngrok agent (not a problem for webrtc stream)
- You can forward multiple ports from a single agent, but you can only run one ngrok agent on the free plan

go2rtc will automatically get your external TCP address (if you enable it in ngrok config) and use it with WebRTC connection (if you enable it in webrtc config).

You need to manually download the [ngrok agent app](https://ngrok.com/download) for your OS and register with the [ngrok service](https://ngrok.com/signup).

**Tunnel for only WebRTC Stream**

You need to add your [ngrok authtoken](https://dashboard.ngrok.com/get-started/your-authtoken) and WebRTC TCP port to YAML:

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

ngrok config example:

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

See the [ngrok agent documentation](https://ngrok.com/docs/agent/config/) for more details on the ngrok configuration file.

### Module: Hass

The best and easiest way to use go2rtc inside the Home Assistant is to install the custom integration [WebRTC Camera](#go2rtc-home-assistant-integration) and custom lovelace card.

But go2rtc is also compatible and can be used with [RTSPtoWebRTC](https://www.home-assistant.io/integrations/rtsp_to_webrtc/) built-in integration.

You have several options on how to add a camera to Home Assistant:

1. Camera RTSP source => [Generic Camera](https://www.home-assistant.io/integrations/generic/)
2. Camera [any source](#module-streams) => [go2rtc config](#configuration) => [Generic Camera](https://www.home-assistant.io/integrations/generic/)
   - Install any [go2rtc](#fast-start)
   - Add your stream to [go2rtc config](#configuration)
   - Hass > Settings > Integrations > Add Integration > [ONVIF](https://my.home-assistant.io/redirect/config_flow_start/?domain=onvif) > Host: `127.0.0.1`, Port: `1984`
   - Hass > Settings > Integrations > Add Integration > [Generic Camera](https://my.home-assistant.io/redirect/config_flow_start/?domain=generic) > Stream Source URL: `rtsp://127.0.0.1:8554/camera1` (change to your stream name, leave everything else as is)

You have several options on how to watch the stream from the cameras in Home Assistant:

1. `Camera Entity` => `Picture Entity Card` => Technology `HLS`, codecs: `H264/H265/AAC`, poor latency.
2. `Camera Entity` => [RTSPtoWebRTC](https://www.home-assistant.io/integrations/rtsp_to_webrtc/) => `Picture Entity Card` => Technology `WebRTC`, codecs: `H264/PCMU/PCMA/OPUS`, best latency.
   - Install any [go2rtc](#fast-start)
   - Hass > Settings > Integrations > Add Integration > [RTSPtoWebRTC](https://my.home-assistant.io/redirect/config_flow_start/?domain=rtsp_to_webrtc) > `http://127.0.0.1:1984/`
   - RTSPtoWebRTC > Configure > STUN server: `stun.l.google.com:19302`
   - Use Picture Entity or Picture Glance lovelace card
3. `Camera Entity` or `Camera URL` => [WebRTC Camera](https://github.com/AlexxIT/WebRTC) => Technology: `WebRTC/MSE/MP4/MJPEG`, codecs: `H264/H265/AAC/PCMU/PCMA/OPUS`, best latency, best compatibility.
   - Install and add [WebRTC Camera](https://github.com/AlexxIT/WebRTC) custom integration
   - Use WebRTC Camera custom lovelace card

You can add camera `entity_id` to [go2rtc config](#configuration) if you need transcoding:

```yaml
streams:
  "camera.hall": ffmpeg:{input}#video=copy#audio=opus
```

**PS.** Default Home Assistant lovelace cards don't support 2-way audio. You can use 2-way audio from [Add-on Web UI](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons). But you need use HTTPS to access the microphone. This is a browser restriction and cannot be avoided.

**PS.** There is also another nice card with go2rtc support - [Frigate Lovelace Card](https://github.com/dermotduffy/frigate-hass-card).

### Module: MP4

Provides several features:

1. MSE stream (fMP4 over WebSocket)
2. Camera snapshots in MP4 format (single frame), can be sent to [Telegram](https://github.com/AlexxIT/go2rtc/wiki/Snapshot-to-Telegram)
3. HTTP progressive streaming (MP4 file stream) - bad format for streaming because of high start delay. This format doesn't work in all Safari browsers, but go2rtc will automatically redirect it to HLS/fMP4 it this case.

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

### Module: HLS

*[New in v1.1.0](https://github.com/AlexxIT/go2rtc/releases/tag/v1.1.0)*

[HLS](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) is the worst technology for real-time streaming. It can only be useful on devices that do not support more modern technology, like [WebRTC](#module-webrtc), [MSE/MP4](#module-mp4).

The go2rtc implementation differs from the standards and may not work with all players.

API examples:

- HLS/TS stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1` (H264)
- HLS/fMP4 stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4` (H264, H265, AAC)

Read more about [codecs filters](#codecs-filters).

### Module: MJPEG

**Important.** For stream as MJPEG format, your source MUST contain the MJPEG codec. If your stream has a MJPEG codec - you can receive **MJPEG stream** or **JPEG snapshots** via API.

You can receive an MJPEG stream in several ways:

- some cameras support MJPEG codec inside [RTSP stream](#source-rtsp) (ex. second stream for Dahua cameras)
- some cameras has HTTP link with [MJPEG stream](#source-http)
- some cameras has HTTP link with snapshots - go2rtc can convert them to [MJPEG stream](#source-http)
- you can convert H264/H265 stream from your camera via [FFmpeg integraion](#source-ffmpeg)

With this example, your stream will have both H264 and MJPEG codecs:

```yaml
streams:
  camera1:
    - rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
    - ffmpeg:camera1#video=mjpeg
```

API examples:

- MJPEG stream: `http://192.168.1.123:1984/api/stream.mjpeg?src=camera1`
- JPEG snapshots: `http://192.168.1.123:1984/api/frame.jpeg?src=camera1`
  - You can use `width`/`w` and/or `height`/`h` params 
  - You can use `rotate` param with `90`, `180`, `270` or `-90` values
  - You can use `hardware`/`hw` param [read more](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration)

**PS.** This module also supports streaming to the server console (terminal) in the **animated ASCII art** format ([read more](https://github.com/AlexxIT/go2rtc/blob/master/internal/mjpeg/README.md)):

[![](https://img.youtube.com/vi/sHj_3h_sX7M/mqdefault.jpg)](https://www.youtube.com/watch?v=sHj_3h_sX7M)

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

By default `go2rtc` starts the Web interface on port `1984` and RTSP on port `8554`, as well as use port `8555` for WebRTC connections. The three ports are accessible from your local network. So anyone on your local network can watch video from your cameras without authorization. The same rule applies to the Home Assistant Add-on.

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
- local access to API is not a problem for [Home Assistant Add-on](#go2rtc-home-assistant-add-on), because Hass runs locally on same server and Add-on Web UI protected with Hass authorization ([Ingress feature](https://www.home-assistant.io/blog/2019/04/15/hassio-ingress/))
- external access to WebRTC TCP port is not a problem, because it used only for transmit encrypted media data
  - anyway you need to open this port to your local network and to the Internet in order for WebRTC to work

If you need Web interface protection without Home Assistant Add-on - you need to use reverse proxy, like [Nginx](https://nginx.org/), [Caddy](https://caddyserver.com/), [ngrok](https://ngrok.com/), etc.

PS. Additionally WebRTC will try to use the 8555 UDP port for transmit encrypted media. It works without problems on the local network. And sometimes also works for external access, even if you haven't opened this port on your router ([read more](https://en.wikipedia.org/wiki/UDP_hole_punching)). But for stable external WebRTC access, you need to open the 8555 port on your router for both TCP and UDP.

## Codecs filters

go2rtc can automatically detect which codecs your device supports for [WebRTC](#module-webrtc) and [MSE](#module-mp4) technologies.

But it cannot be done for [RTSP](#module-rtsp), [HTTP progressive streaming](#module-mp4), [HLS](#module-hls) technologies. You can manually add a codec filter when you create a link to a stream. The filters work the same for all three technologies. Filters do not create a new codec. They only select the suitable codec from existing sources. You can add new codecs to the stream using the [FFmpeg transcoding](#source-ffmpeg).

Without filters:

- RTSP will provide only the first video and only the first audio (any codec)
- MP4 will include only compatible codecs (H264, H265, AAC)
- HLS will output in the legacy TS format (H264 without audio)

Some examples:

- `rtsp://192.168.1.123:8554/camera1?mp4` - useful for recording as MP4 files (e.g. Hass or Frigate)
- `rtsp://192.168.1.123:8554/camera1?video=h264,h265&audio=aac` - full version of the filter above
- `rtsp://192.168.1.123:8554/camera1?video=h264&audio=aac&audio=opus` - H264 video codec and two separate audio tracks
- `rtsp://192.168.1.123:8554/camera1?video&audio=all` - any video codec and all audio codecs as separate tracks
- `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4` - HLS stream with MP4 compatible codecs (HLS/fMP4)
- `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4=flac` - HLS stream with PCMA/PCMU/PCM audio support (HLS/fMP4), won't work on old devices
- `http://192.168.1.123:1984/api/stream.mp4?src=camera1&mp4=flac` - MP4 file with PCMA/PCMU/PCM audio support, won't work on old devices (ex. iOS 12)
- `http://192.168.1.123:1984/api/stream.mp4?src=camera1&mp4=all` - MP4 file with non standard audio codecs, won't work on some players

## Codecs madness

`AVC/H.264` video can be played almost anywhere. But `HEVC/H.265` has a lot of limitations in supporting with different devices and browsers. It's all about patents and money, you can't do anything about it.

| Device                                                                   | WebRTC                                  | MSE                                     | HTTP*                                        | HLS                         |
|--------------------------------------------------------------------------|-----------------------------------------|-----------------------------------------|----------------------------------------------|-----------------------------|
| *latency*                                                                | best                                    | medium                                  | bad                                          | bad                         |
| - Desktop Chrome 107+ <br/> - Desktop Edge <br/> - Android Chrome 107+   | H264 <br/> PCMU, PCMA <br/> OPUS        | H264, H265* <br/> AAC, FLAC* <br/> OPUS | H264, H265* <br/> AAC, FLAC* <br/> OPUS, MP3 | no                          |
| Desktop Firefox                                                          | H264 <br/> PCMU, PCMA <br/> OPUS        | H264 <br/> AAC, FLAC* <br/> OPUS        | H264 <br/> AAC, FLAC* <br/> OPUS             | no                          |
| - Desktop Safari 14+ <br/> - iPad Safari 14+ <br/> - iPhone Safari 17.1+ | H264, H265* <br/> PCMU, PCMA <br/> OPUS | H264, H265 <br/> AAC, FLAC*             | **no!**                                      | H264, H265 <br/> AAC, FLAC* |
| iPhone Safari 14+                                                        | H264, H265* <br/> PCMU, PCMA <br/> OPUS | **no!**                                 | **no!**                                      | H264, H265 <br/> AAC, FLAC* |
| macOS [Hass App][1]                                                      | no                                      | no                                      | no                                           | H264, H265 <br/> AAC, FLAC* |

[1]: https://apps.apple.com/app/home-assistant/id1099568401

`HTTP*` - HTTP Progressive Streaming, not related with [Progressive download](https://en.wikipedia.org/wiki/Progressive_download), because the file has no size and no end 

- Chrome H265: [read this](https://chromestatus.com/feature/5186511939567616) and [read this](https://github.com/StaZhu/enable-chromium-hevc-hardware-decoding)
- Edge H265: [read this](https://www.reddit.com/r/MicrosoftEdge/comments/v9iw8k/enable_hevc_support_in_edge/)
- Desktop Safari H265: Menu > Develop > Experimental > WebRTC H265
- iOS Safari H265: Settings > Safari > Advanced > Experimental > WebRTC H265

**Audio**

- Go2rtc support [automatic repack](#built-in-transcoding) `PCMA/PCMU/PCM` codecs to `FLAC` for MSE/MP4/HLS so they will work almost anywhere
- **WebRTC** audio codecs: `PCMU/8000`, `PCMA/8000`, `OPUS/48000/2`
- `OPUS` and `MP3` inside **MP4** is part of the standard, but some players do not support them anyway (especially Apple)

**Apple devices**

- all Apple devices don't support HTTP progressive streaming
- iPhones don't support MSE technology because it competes with the HTTP Live Streaming (HLS) technology, invented by Apple
- HLS is the worst technology for **live** streaming, it still exists only because of iPhones

**Codec names**

- H264 = H.264 = AVC (Advanced Video Coding)
- H265 = H.265 = HEVC (High Efficiency Video Coding)
- PCMA = G.711 PCM (A-law) = PCM A-law (`alaw`)
- PCMU = G.711 PCM (µ-law) = PCM mu-law (`mulaw`)
- PCM = L16 = PCM signed 16-bit big-endian (`s16be`)
- AAC = MPEG4-GENERIC
- MP3 = MPEG-1 Audio Layer III or MPEG-2 Audio Layer III

## Built-in transcoding

There are no plans to embed complex transcoding algorithms inside go2rtc. [FFmpeg source](#source-ffmpeg) does a great job with this. Including [hardware acceleration](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration) support.

But go2rtc has some simple algorithms. They are turned on automatically, you do not need to set them up additionally.

**PCM for MSE/MP4/HLS**

Go2rtc can pack `PCMA`, `PCMU` and `PCM` codecs into an MP4 container so that they work in all browsers and all built-in players on modern devices. Including Apple QuickTime:

```
PCMA/PCMU => PCM => FLAC => MSE/MP4/HLS
```

**Resample PCMA/PCMU for WebRTC**

By default WebRTC support only `PCMA/8000` and `PCMU/8000`. But go2rtc can automatically resample PCMA and PCMU codec with with a different sample rate. Also go2rtc can transcode `PCM` codec to `PCMA/8000`, so WebRTC can play it:

```
PCM/xxx => PCMA/8000 => WebRTC
PCMA/xxx => PCMA/8000 => WebRTC
PCMU/xxx => PCMU/8000 => WebRTC
```

**Important**

- FLAC codec not supported in a RTSP stream. If you using Frigate or Hass for recording MP4 files with PCMA/PCMU/PCM audio - you should setup transcoding to AAC codec.
- PCMA and PCMU are VERY low quality codecs. Them support only 256! different sounds. Use them only when you have no other options.

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

![](assets/codecs.svg)

**PS.** You can select `PCMU` or `PCMA` codec in camera setting and don't use transcoding at all. Or you can select `AAC` codec for main stream and `PCMU` codec for second stream and add both RTSP to YAML config, this also will work fine.

## Projects using go2rtc

- [Frigate 12+](https://frigate.video/) - open source NVR built around real-time AI object detection
- [Frigate Lovelace Card](https://github.com/dermotduffy/frigate-hass-card) - custom card for Home Assistant
- [OpenIPC](https://github.com/OpenIPC/firmware/tree/master/general/package/go2rtc) - Alternative IP Camera firmware from an open community
- [wz_mini_hacks](https://github.com/gtxaspec/wz_mini_hacks) - Custom firmware for Wyze cameras
- [EufyP2PStream](https://github.com/oischinger/eufyp2pstream) - A small project that provides a Video/Audio Stream from Eufy cameras that don't directly support RTSP
- [ioBroker.euSec](https://github.com/bropat/ioBroker.eusec) - [ioBroker](https://www.iobroker.net/) adapter for control Eufy security devices
- [MMM-go2rtc](https://github.com/Anonym-tsk/MMM-go2rtc) - MagicMirror² Module
- [ring-mqtt](https://github.com/tsightler/ring-mqtt) - Ring devices to MQTT Bridge

**Distributions**

- [Alpine Linux](https://pkgs.alpinelinux.org/packages?name=go2rtc)
- [Arch User Repository](https://linux-packages.com/aur/package/go2rtc)
- [Gentoo](https://github.com/inode64/inode64-overlay/tree/main/media-video/go2rtc)
- [NixOS](https://search.nixos.org/packages?query=go2rtc)
- [Proxmox Helper Scripts](https://tteck.github.io/Proxmox/)
- [QNAP](https://www.myqnap.org/product/go2rtc/)
- [Synology NAS](https://synocommunity.com/package/go2rtc)
- [Unraid](https://unraid.net/community/apps?q=go2rtc)

## Cameras experience

- [Dahua](https://www.dahuasecurity.com/) - reference implementation streaming protocols, a lot of settings, high stream quality, multiple streaming clients
- [EZVIZ](https://www.ezviz.com/) - awful RTSP protocol realisation, many bugs in SDP
- [Hikvision](https://www.hikvision.com/) - a lot of proprietary streaming technologies
- [Reolink](https://reolink.com/) - some models has awful unusable RTSP realisation and not best RTMP alternative (I recommend that you contact Reolink support for new firmware), few settings
- [Sonoff](https://sonoff.tech/) - very low stream quality, no settings, not best protocol implementation
- [TP-Link](https://www.tp-link.com/) - few streaming clients, packet loss?
- Chinese cheap noname cameras, Wyze Cams, Xiaomi cameras with hacks (usual has `/live/ch00_1` in RTSP URL) - awful but usable RTSP protocol realisation, low stream quality, few settings, packet loss?

## TIPS

**Using apps for low RTSP delay**

- `ffplay -fflags nobuffer -flags low_delay "rtsp://192.168.1.123:8554/camera1"`
- VLC > Preferences > Input / Codecs > Default Caching Level: Lowest Latency

**Snapshots to Telegram**

[read more](https://github.com/AlexxIT/go2rtc/wiki/Snapshot-to-Telegram)

## FAQ

**Q. What's the difference between go2rtc, WebRTC Camera and RTSPtoWebRTC?**

**go2rtc** is a new version of the server-side [WebRTC Camera](https://github.com/AlexxIT/WebRTC) integration, completely rewritten from scratch, with a number of fixes and a huge number of new features. It is compatible with native Home Assistant [RTSPtoWebRTC](https://www.home-assistant.io/integrations/rtsp_to_webrtc/) integration. So you [can use](#module-hass) default lovelace Picture Entity or Picture Glance.

**Q. Should I use go2rtc addon or WebRTC Camera integration?**

**go2rtc** is more than just viewing your stream online with WebRTC/MSE/HLS/etc. You can use it all the time for your various tasks. But every time the Hass is rebooted - all integrations are also rebooted. So your streams may be interrupted if you use them in additional tasks.

Basic users can use **WebRTC Camera** integration. Advanced users can use go2rtc addon or Frigate 12+ addon.

**Q. Which RTSP link should I use inside Hass?**

You can use direct link to your cameras there (as you always do). **go2rtc** support zero-config feature. You may leave `streams` config section empty. And your streams will be created on the fly on first start from Hass. And your cameras will have multiple connections. Some from Hass directly and one from **go2rtc**.

Also you can specify your streams in **go2rtc** [config file](#configuration) and use RTSP links to this addon. With additional features: multi-source [codecs negotiation](#codecs-negotiation) or FFmpeg [transcoding](#source-ffmpeg) for unsupported codecs. Or use them as source for Frigate. And your cameras will have one connection from **go2rtc**. And **go2rtc** will have multiple connection - some from Hass via RTSP protocol, some from your browser via WebRTC/MSE/HLS protocols.

Use any config what you like.

**Q. What about lovelace card with support 2-way audio?**

At this moment I am focused on improving stability and adding new features to **go2rtc**. Maybe someone could write such a card themselves. It's not difficult, I have [some sketches](https://github.com/AlexxIT/go2rtc/blob/master/www/webrtc.html).
