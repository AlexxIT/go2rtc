<h1 align="center">
  <a href="https://github.com/AlexxIT/go2rtc">
    <img src="./website/images/logo.gif" alt="go2rtc - GitHub">
  </a>
</h1>
<p align="center">
  <a href="https://github.com/AlexxIT/go2rtc/stargazers" target="_blank">
    <img style="display: inline" src="https://img.shields.io/github/stars/AlexxIT/go2rtc?style=flat-square&logo=github" alt="go2rtc - GitHub Stars">
  </a>
  <a href="https://hub.docker.com/r/alexxit/go2rtc" target="_blank">
    <img style="display: inline" src="https://img.shields.io/docker/pulls/alexxit/go2rtc?style=flat-square&logo=docker&logoColor=white&label=pulls" alt="go2rtc - Docker Pulls">
  </a>
  <a href="https://github.com/AlexxIT/go2rtc/releases" target="_blank">
    <img style="display: inline" src="https://img.shields.io/github/downloads/AlexxIT/go2rtc/total?color=blue&style=flat-square&logo=github" alt="go2rtc - GitHub Downloads">
  </a>
</p>
<p align="center">
  <a href="https://trendshift.io/repositories/4628" target="_blank">
    <img src="https://trendshift.io/api/badge/repositories/4628" alt="go2rtc - Trendshift"/>
  </a>
</p>

Ultimate camera streaming application with support for dozens formats and protocols.

- zero-dependency [small app](#go2rtc-binary) for all OS (Windows, macOS, Linux, FreeBSD)
- zero-delay for many [supported protocols](#codecs-madness) (lowest possible streaming latency)
- [streaming input](#streaming-input) from dozens formats and protocols
- [streaming output](#streaming-output) in all popular formats
- [streaming ingest](#streaming-ingest) in a number of popular formats
- [publish](#publish-stream) any source to popular streaming services (YouTube, Telegram)
- on-the-fly transcoding only if necessary via [FFmpeg](internal/ffmpeg/README.md)
- [two-way audio](#two-way-audio) support for many formats
- [streaming audio](#stream-to-camera) to all cameras with [two-way audio](#two-way-audio) support
- mixing tracks from different sources to single stream
- [auto-match](www/README.md#javascript-api) client-supported streaming formats and codecs
- [streaming stats](#streaming-stats) for all active connections
- can be [integrated to any project](#projects-using-go2rtc) or be used as [standalone app](#go2rtc-binary)

#### Inspired by

- series of streaming projects from [@deepch](https://github.com/deepch)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [rtsp-simple-server](https://github.com/aler9/rtsp-simple-server) idea from [@aler9](https://github.com/aler9)
- [GStreamer](https://gstreamer.freedesktop.org/) framework pipeline idea
- [MediaSoup](https://mediasoup.org/) framework routing idea
- HomeKit Accessory Protocol from [@brutella](https://github.com/brutella/hap)
- creator of the project's logo [@v_novoseltsev](https://www.instagram.com/v_novoseltsev)

<br>
<details>
<summary><b>Table of Contents</b></summary>

- [Installation](#installation)
  - [go2rtc: Binary](#go2rtc-binary)
  - [go2rtc: Docker](#go2rtc-docker)
  - [go2rtc: Home Assistant add-on](#go2rtc-home-assistant-add-on)
  - [go2rtc: Home Assistant Integration](#go2rtc-home-assistant-integration)
  - [go2rtc: Master version](#go2rtc-master-version)
- [Configuration](#configuration)
- [Features](#features)
  - [Streaming input](#streaming-input)
  - [Streaming output](#streaming-output)
  - [Streaming ingest](#streaming-ingest)
  - [Two-way audio](#two-way-audio)
  - [Stream to camera](#stream-to-camera)
  - [Publish stream](#publish-stream)
  - [Preload stream](#preload-stream)
  - [Streaming stats](#streaming-stats)
- [Codecs](#codecs)
  - [Codecs filters](#codecs-filters)
  - [Codecs madness](#codecs-madness)
  - [Built-in transcoding](#built-in-transcoding)
  - [Codecs negotiation](#codecs-negotiation)
- [Security](#security)
- [Projects using go2rtc](#projects-using-go2rtc)
- [Camera experience](#camera-experience)
- [Tips](#tips)

</details>

## Installation

1. Download [binary](#go2rtc-binary) or use [Docker](#go2rtc-docker) or Home Assistant [add-on](#go2rtc-home-assistant-add-on) or [integration](#go2rtc-home-assistant-integration)
2. Open web interface: `http://localhost:1984/`
3. Add [streams](#streaming-input) to [config](#configuration)

**Developers:** integrate [HTTP API](internal/api/README.md) into your smart home platform.

### go2rtc: Binary

Download binary for your OS from [latest release](https://github.com/AlexxIT/go2rtc/releases/):

| name                                                                                                            | description                                                                                                                               |
|-----------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| [go2rtc_win64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_win64.zip)                 | Windows 10+ 64-bit                                                                                                                        |
| [go2rtc_win32.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_win32.zip)                 | Windows 10+ 32-bit                                                                                                                        |
| [go2rtc_win_arm64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_win_arm64.zip)         | Windows ARM 64-bit                                                                                                                        |
| [go2rtc_linux_amd64](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_amd64)             | Linux 64-bit                                                                                                                              |
| [go2rtc_linux_i386](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_i386)               | Linux 32-bit                                                                                                                              |
| [go2rtc_linux_arm64](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_arm64)             | Linux ARM 64-bit (ex. Raspberry 64-bit OS)                                                                                                |
| [go2rtc_linux_arm](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_arm)                 | Linux ARM 32-bit (ex. Raspberry 32-bit OS)                                                                                                |
| [go2rtc_linux_armv6](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_armv6)             | Linux ARMv6 (for old Raspberry 1 and Zero)                                                                                                |
| [go2rtc_linux_mipsel](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_mipsel)           | Linux MIPS (ex. [Xiaomi Gateway 3](https://github.com/AlexxIT/XiaomiGateway3), [Wyze cameras](https://github.com/gtxaspec/wz_mini_hacks)) |
| [go2rtc_mac_amd64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_mac_amd64.zip)         | macOS 11+ Intel 64-bit                                                                                                                    |
| [go2rtc_mac_arm64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_mac_arm64.zip)         | macOS ARM 64-bit                                                                                                                          |
| [go2rtc_freebsd_amd64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_freebsd_amd64.zip) | FreeBSD 64-bit                                                                                                                            |
| [go2rtc_freebsd_arm64.zip](https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_freebsd_arm64.zip) | FreeBSD ARM 64-bit                                                                                                                        |

Don't forget to fix the rights `chmod +x go2rtc_xxx_xxx` on Linux and Mac.

PS. The application is compiled with the latest versions of the Go language for maximum speed and security. Therefore, the [minimum OS versions](https://go.dev/wiki/MinimumRequirements) depend on the Go language.

### go2rtc: Docker

The Docker containers [`alexxit/go2rtc`](https://hub.docker.com/r/alexxit/go2rtc) and [`ghcr.io/alexxit/go2rtc`](https://github.com/AlexxIT/go2rtc/pkgs/container/go2rtc) support multiple architectures including `386`, `amd64`, `arm/v6`, `arm/v7` and `arm64`.
These containers offer the same functionality as the Home Assistant [add-on](#go2rtc-home-assistant-add-on) but are designed to operate independently of Home Assistant.
It comes preinstalled with [FFmpeg](internal/ffmpeg/README.md) and [Python](internal/echo/README.md).

### go2rtc: Home Assistant add-on

[![Open your Home Assistant instance and show the add add-on repository dialog with a specific repository URL pre-filled.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons)

1. Settings > Add-ons > Plus > Repositories > Add
   ```
   https://github.com/AlexxIT/hassio-addons
   ```
2. go2rtc > Install > Start

### go2rtc: Home Assistant Integration

[WebRTC Camera](https://github.com/AlexxIT/WebRTC) custom component can be used on any Home Assistant [installation](https://www.home-assistant.io/installation/), including [HassWP](https://github.com/AlexxIT/HassWP) on Windows. It can automatically download and use the latest version of go2rtc. Or it can connect to an existing version of go2rtc. Addon installation in this case is optional.

### go2rtc: Master version

Latest, but maybe unstable version:

- Binary: [latest master build](https://nightly.link/AlexxIT/go2rtc/workflows/build/master)
- Docker: `alexxit/go2rtc:master` or `alexxit/go2rtc:master-hardware` versions
- Home Assistant add-on: `go2rtc master` or `go2rtc master hardware` versions

## Configuration

This is the `go2rtc.yaml` file in [YAML-format](https://en.wikipedia.org/wiki/YAML).
The configuration can be changed in the [WebUI](www/README.md) at `http://localhost:1984`.
The editor provides syntax highlighting and checking.

![go2rtc webui config](website/images/webui-config.png)

The simplest config looks like this:

```yaml
streams:
  hall-camera: rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
```

- by default go2rtc will search `go2rtc.yaml` in the current work directory
- `api` server will start on default **1984 port** (TCP)
- `rtsp` server will start on default **8554 port** (TCP)
- `webrtc` will use port **8555** (TCP/UDP) for connections

More information can be [found here](internal/app/README.md).

## Features

A summary table of all modules and features can be found [here](internal/README.md).

**Core modules**

- [`app`](internal/app/README.md) - Reading [configs](internal/app/README.md) and setting up [logs](internal/app/README.md#log).
- [`api`](internal/api/README.md) - Handle [HTTP](internal/api/README.md) and [WebSocket](internal/api/ws/README.md) API.
- [`streams`](internal/streams/README.md) - Handle a list of streams.

### Streaming input

#### public protocols

- [`mpjpeg`](internal/mjpeg/README.md#mjpeg-client) - The legacy but still used [MJPEG](https://en.wikipedia.org/wiki/Motion_JPEG) protocol for real-time media transmission.
- [`onvif`](internal/onvif/README.md#onvif-client) - A popular [ONVIF](https://en.wikipedia.org/wiki/ONVIF) protocol for receiving media in RTSP format.
- [`rtmp`](internal/rtmp/README.md#rtmp-client) - The legacy but still used [RTMP](https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol) protocol for real-time media transmission.
- [`rtsp`](internal/rtsp/README.md#rtsp-client) - The most common [RTSP](https://en.wikipedia.org/wiki/Real-Time_Streaming_Protocol) protocol for real-time media transmission.
- [`webrtc`](internal/webrtc/README.md#webrtc-client) - [WebRTC](https://en.wikipedia.org/wiki/WebRTC) web-compatible protocol for real-time media transmission.
- [`yuv4mpegpipe`](internal/http/README.md#tcp) - Raw [YUV](https://en.wikipedia.org/wiki/Y%E2%80%B2UV) frame stream with [YUV4MPEG](https://manned.org/yuv4mpeg) header.

#### private protocols

- [`bubble`](internal/bubble/README.md) - Some NVR from [dvr163.com](http://help.dvr163.com/) and [eseecloud.com](http://www.eseecloud.com/).
- [`doorbird`](internal/doorbird/README.md) - [Doorbird](https://www.doorbird.com/) devices with two-way audio.
- [`dvrip`](internal/dvrip/README.md) - DVR-IP NVR, NetSurveillance, Sofia protocol (XMeye SDK).
- [`eseecloud`](internal/eseecloud/README.md) - Some NVR from [dvr163.com](http://help.dvr163.com/) and [eseecloud.com](http://www.eseecloud.com/).
- [`gopro`](internal/gopro/README.md) - [GoPro](https://gopro.com/) cameras, connected via USB or Wi-Fi.
- [`hass`](internal/hass/README.md) - Import cameras from [Home Assistant](https://www.home-assistant.io/) config files.
- [`homekit`](internal/homekit/README.md) - Cameras with [Apple HomeKit](https://www.apple.com/home-app/accessories/) protocol.
- [`isapi`](internal/isapi/README.md) - Two-way audio for [Hikvision ISAPI](https://tpp.hikvision.com/download/ISAPI_OTAP) protocol.
- [`kasa`](internal/kasa/README.md) - [TP-Link Kasa](https://www.kasasmart.com/) cameras.
- [`multitrans`](internal/multitrans/README.md) - Two-way audio for Chinese version of [TP-Link](https://www.tp-link.com.cn/) cameras.
- [`nest`](internal/nest/README.md) - [Google Nest](https://developers.google.com/nest/device-access/supported-devices) cameras through user-unfriendly and paid APIs.
- [`ring`](internal/ring/README.md) - Ring cameras with two-way audio support.
- [`roborock`](internal/roborock/README.md) - [Roborock](https://roborock.com/) vacuums with cameras with two-way audio support. 
- [`tapo`](internal/tapo/README.md) - [TP-Link Tapo](https://www.tapo.com/) cameras with two-way audio support.
- [`vigi`](internal/tapo/README.md#tp-link-vigi) - TP-Link Vigi cameras.
- [`tuya`](internal/tuya/README.md) - [Tuya](https://www.tuya.com/) ecosystem cameras with two-way audio support.
- [`webtorrent`](internal/webtorrent/README.md) - Stream from another go2rtc via [WebTorrent](https://en.wikipedia.org/wiki/WebTorrent) protocol.
- [`wyze`](internal/wyze/README.md) - [Wyze](https://wyze.com/) cameras using native P2P protocol
- [`xiaomi`](internal/xiaomi/README.md) - [Xiaomi Mi Home](https://home.mi.com/) ecosystem cameras with two-way audio support.

#### devices

- [`alsa`](internal/alsa/README.md) - A [framework](https://en.wikipedia.org/wiki/Advanced_Linux_Sound_Architecture) for receiving audio from devices on Linux OS.
- [`v4l2`](internal/v4l2/README.md) - A [framework](https://en.wikipedia.org/wiki/Video4Linux) for receiving video from devices on Linux OS.

#### files

- [`adts`](internal/http/README.md#tcp) - Audio stream in [AAC](https://en.wikipedia.org/wiki/Advanced_Audio_Coding) codec with Audio Data Transport Stream headers.
- [`flv`](internal/http/README.md#tcp) - The legacy but still used [Flash Video](https://en.wikipedia.org/wiki/Flash_Video) format.
- [`h264`](internal/http/README.md#tcp) - AVC/H.264 bitstream.
- [`hevc`](internal/http/README.md#tcp) - HEVC/H.265 bitstream.
- [`hls`](internal/http/README.md) - A popular [HTTP Live Streaming](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) format.
- [`mjpeg`](internal/http/README.md#tcp) - A continuous sequence of JPEG frames (without HTTP headers).
- [`mpegts`](internal/http/README.md#tcp) - The legacy [MPEG transport stream](https://en.wikipedia.org/wiki/MPEG_transport_stream) format.
- [`wav`](internal/http/README.md#tcp) - Audio stream in [Waveform Audio File](https://en.wikipedia.org/wiki/WAV) format.

#### scripts

- [`echo`](internal/echo/README.md) - If the source has a dynamic link, you can use a bash or python script to get it.
- [`exec`](internal/exec/README.md) - You can run an external application (`ffmpeg`, `gstreamer`, `rpicam`, etc.) and receive a media stream from it.
- [`expr`](internal/expr/README.md) - If the source has a dynamic link, you can use [Expr](https://github.com/expr-lang/expr) language to get it.
- [`ffmpeg`](internal/ffmpeg/README.md) - Use [FFmpeg](https://ffmpeg.org/) as a stream source. Hardware-accelerated transcoding and streaming from USB devices are supported.

#### webrtc

- [`creality`](internal/webrtc/README.md#creality) - [Creality](https://www.creality.com/) 3D printer cameras.
- [`kinesis`](internal/webrtc/README.md#kinesis) - [Amazon Kinesis](https://aws.amazon.com/kinesis/video-streams/) video streams.
- [`openipc`](internal/webrtc/README.md#openipc) - Cameras on open-source [OpenIPC](https://openipc.org/) firmware.
- [`switchbot`](internal/webrtc/README.md#switchbot) - [SwitchBot](https://us.switch-bot.com/) cameras.
- [`whep`](internal/webrtc/README.md#whep) - [WebRTC/WHEP](https://datatracker.ietf.org/doc/draft-murillo-whep/) is replaced by [WebRTC/WISH](https://datatracker.ietf.org/doc/charter-ietf-wish/02/) standard for WebRTC video/audio viewers.
- [`wyze`](internal/webrtc/README.md#wyze) - Legacy method to connect to [Wyze](https://www.wyze.com/) cameras via [docker-wyze-bridge](https://github.com/mrlt8/docker-wyze-bridge).

### Streaming output

- [`adts`](internal/mpeg/README.md) - Output stream in ADTS format with [AAC](https://en.wikipedia.org/wiki/Advanced_Audio_Coding) audio.
- [`ascii`](internal/mjpeg/README.md#ascii) - Just for fun stream as [ASCII to Terminal](https://www.youtube.com/watch?v=sHj_3h_sX7M).
- [`flv`](internal/rtmp/README.md) - Output stream in [Flash Video](https://en.wikipedia.org/wiki/Flash_Video) format.
- [`hls`](internal/hls/README.md) - Output stream in [HTTP Live Streaming](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) format.
- [`homekit`](internal/homekit/README.md#homekit-server) - Output stream to [Apple Home](https://www.apple.com/home-app/) using [HomeKit](https://en.wikipedia.org/wiki/Apple_Home) protocol.
- [`jpeg`](internal/mjpeg/README.md#jpeg) - Output snapshots in [JPEG](https://en.wikipedia.org/wiki/JPEG) format.
- [`mpjpeg`](internal/mjpeg/README.md#mpjpeg) - Output a stream in [MJPEG](https://en.wikipedia.org/wiki/Motion_JPEG) format.
- [`mp4`](internal/mp4/README.md) - Output as [MP4 stream](https://en.wikipedia.org/wiki/Progressive_download) or [Media Source Extensions](https://developer.mozilla.org/en-US/docs/Web/API/Media_Source_Extensions_API) (MSE) compatible format.
- [`mpegts`](internal/mpeg/README.md) - Output stream in [MPEG transport stream](https://en.wikipedia.org/wiki/MPEG_transport_stream) format.
- [`onvif`](internal/onvif/README.md#onvif-server) - Output stream using [ONVIF](https://en.wikipedia.org/wiki/ONVIF) protocol.
- [`rtmp`](internal/rtmp/README.md#rtmp-server) - Output stream using [Real-Time Messaging](https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol) protocol.
- [`rtsp`](internal/rtsp/README.md#rtsp-server) - Output stream using [Real-Time Streaming](https://en.wikipedia.org/wiki/Real-Time_Streaming_Protocol) protocol.
- [`webrtc`](internal/webrtc/README.md#webrtc-server) - Output stream using [Web Real-Time Communication](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API) API.
- [`webtorrent`](internal/webtorrent/README.md#webtorrent-server) - Output stream using [WebTorrent](https://en.wikipedia.org/wiki/WebTorrent) protocol.
- [`yuv4mpegpipe`](internal/mjpeg/README.md#yuv4mpegpipe) - Output in raw [YUV](https://en.wikipedia.org/wiki/Y%E2%80%B2UV) frame stream with [YUV4MPEG](https://manned.org/yuv4mpeg) header.

### Streaming ingest

Supported for: 
[`flv`](internal/rtmp/README.md#flv-server), 
[`mjpeg`](internal/mjpeg/README.md#streaming-ingest), 
[`mpegts`](internal/mpeg/README.md#streaming-ingest), 
[`rtmp`](internal/rtmp/README.md#rtmp-server), 
[`rtsp`](internal/rtsp/README.md#streaming-ingest), 
[`webrtc`](internal/webrtc/README.md#streaming-ingest).

This is a feature when go2rtc expects to receive an incoming stream from an external application. The stream transmission is started and stopped by an external application.

- You can push data only to an existing stream (create a stream with empty source in config).
- You can push multiple incoming sources to the same stream.
- You can push data to a non-empty stream, so it will have additional codecs inside.

### Two-way audio

Supported for:
[`doorbird`](internal/doorbird/README.md), 
[`dvrip`](internal/dvrip/README.md), 
[`exec`](internal/exec/README.md), 
[`isapi`](internal/isapi/README.md), 
[`multitrans`](internal/multitrans/README.md), 
[`ring`](internal/ring/README.md), 
[`roborock`](internal/roborock/README.md), 
[`rtsp`](internal/rtsp/README.md#two-way-audio), 
[`tapo`](internal/tapo/README.md), 
[`tuya`](internal/tuya/README.md), 
[`webrtc`](internal/webrtc/README.md), 
[`wyze`](internal/wyze/README.md), 
[`xiaomi`](internal/xiaomi/README.md).

Two-way audio can be used in browser with [WebRTC](internal/webrtc/README.md) technology. The browser will give access to the microphone only for HTTPS sites ([read more](https://stackoverflow.com/questions/52759992/how-to-access-camera-and-microphone-in-chrome-without-https)).

### Stream to camera

You can play audio files or live streams on any camera with [two-way audio](#two-way-audio) support.

[read more](internal/streams/README.md#stream-to-camera)

### Publish stream

You can publish any stream to streaming services (YouTube, Telegram, etc.) via RTMP/RTMPS.

[read more](internal/streams/README.md#publish-stream)

### Preload stream

You can preload any stream on go2rtc start. This is useful for cameras that take a long time to start up.

[read more](internal/streams/README.md#preload-stream)

### Streaming stats

[WebUI](www/README.md) provides detailed information about all active connections, including IP-addresses, formats, protocols, number of packets and bytes transferred. 
Via the [HTTP API](internal/api/README.md) in [`json`](https://en.wikipedia.org/wiki/JSON) or [`dot`](https://en.wikipedia.org/wiki/DOT_(graph_description_language)) format on an interactive connection map.

![go2rtc webui net](website/images/webui-net.png)

## Codecs

If you have questions about why video or audio is not displayed, you need to read the following sections.

| Name                         | FFmpeg   | RTSP          | Aliases     |
|------------------------------|----------|---------------|-------------|
| Advanced Audio Coding        | `aac`    | MPEG4-GENERIC |             |
| Advanced Video Coding        | `h264`   | H264          | AVC, H.264  |
| G.711 PCM (A-law)            | `alaw`   | PCMA          | G711A       |
| G.711 PCM (µ-law)            | `mulaw`  | PCMU          | G711u       |
| High Efficiency Video Coding | `hevc`   | H265          | HEVC, H.265 |
| Motion JPEG                  | `mpjpeg` | JPEG          |             |
| MPEG-1 Audio Layer III       | `mp3`    | MPA           |             |
| Opus Codec                   | `opus`   | OPUS          |             |
| PCM signed 16-bit big-endian | `s16be`  | L16           |             |

### Codecs filters

go2rtc can automatically detect which codecs your device supports for [WebRTC](internal/webrtc/README.md) and [MSE](internal/mp4/README.md) technologies.

But it cannot be done for [RTSP](internal/rtsp/README.md), [HTTP progressive streaming](internal/mp4/README.md), [HLS](internal/hls/README.md) technologies. 
You can manually add a codec filter when you create a link to a stream. 
The filters work the same for all three technologies. 
Filters do not create a new codec, they only select the suitable codec from existing sources. 
You can add new codecs to the stream using the [FFmpeg transcoding](internal/ffmpeg/README.md).

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

### Codecs madness

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

### Built-in transcoding

There are no plans to embed complex transcoding algorithms inside go2rtc. 
[FFmpeg source](internal/ffmpeg/README.md) does a great job with this. 
Including [hardware acceleration](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration) support.

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

### Codecs negotiation

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

**PS.** You can select `PCMU` or `PCMA` codec in camera settings and not use transcoding at all. Or you can select `AAC` codec for main stream and `PCMU` codec for second stream and add both RTSP to YAML config, this also will work fine.

## Security

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

- local access to RTSP is not a problem for [FFmpeg](internal/ffmpeg/README.md) integration, because it runs locally on your server
- local access to API is not a problem for the [Home Assistant add-on](#go2rtc-home-assistant-add-on), because Home Assistant runs locally on the same server, and the add-on web UI is protected with Home Assistant authorization ([Ingress feature](https://www.home-assistant.io/blog/2019/04/15/hassio-ingress/))
- external access to WebRTC TCP port is not a problem, because it is used only for transmitting encrypted media data
    - anyway you need to open this port to your local network and to the Internet for WebRTC to work

If you need web interface protection without the Home Assistant add-on, you need to use a reverse proxy, like [Nginx](https://nginx.org/), [Caddy](https://caddyserver.com/), etc.

PS. Additionally, WebRTC will try to use the 8555 UDP port to transmit encrypted media. It works without problems on the local network, and sometimes also works for external access, even if you haven't opened this port on your router ([read more](https://en.wikipedia.org/wiki/UDP_hole_punching)). But for stable external WebRTC access, you need to open the 8555 port on your router for both TCP and UDP.

## Projects using go2rtc

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

## Camera experience

- [Dahua](https://www.dahuasecurity.com/) - reference implementation streaming protocols, a lot of settings, high stream quality, multiple streaming clients
- [EZVIZ](https://www.ezviz.com/) - awful RTSP protocol implementation, many bugs in SDP
- [Hikvision](https://www.hikvision.com/) - a lot of proprietary streaming technologies
- [Reolink](https://reolink.com/) - some models have an awful, unusable RTSP implementation and not the best RTMP alternative (I recommend that you contact Reolink support for new firmware), few settings
- [Sonoff](https://sonoff.tech/) - very low stream quality, no settings, not the best protocol implementation
- [TP-Link](https://www.tp-link.com/) - few streaming clients, packet loss?
- Cheap noname cameras, Wyze Cams, Xiaomi cameras with hacks (usually have `/live/ch00_1` in RTSP URL) - awful but usable RTSP protocol implementation, low stream quality, few settings, packet loss?

## Tips

**Using apps for low RTSP delay**

- `ffplay -fflags nobuffer -flags low_delay "rtsp://192.168.1.123:8554/camera1"`
- VLC > Preferences > Input / Codecs > Default Caching Level: Lowest Latency

**Snapshots to Telegram**

[read more](https://github.com/AlexxIT/go2rtc/wiki/Snapshot-to-Telegram)
