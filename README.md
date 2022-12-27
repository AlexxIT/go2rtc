# go2rtc

Ultimate camera streaming application with support RTSP, WebRTC, HomeKit, FFmpeg, RTMP, etc.

![](assets/go2rtc.png)

- zero-dependency and zero-config [small app](#go2rtc-binary) for all OS (Windows, macOS, Linux, ARM)
- zero-delay for many supported protocols (lowest possible streaming latency)
- streaming from [RTSP](#source-rtsp), [RTMP](#source-rtmp), [HTTP](#source-http) (FLV/MJPEG/JPEG), [FFmpeg](#source-ffmpeg), [USB Cameras](#source-ffmpeg-device) and [other sources](#module-streams)
- streaming to [RTSP](#module-rtsp), [WebRTC](#module-webrtc), [MSE/MP4](#module-mp4) or [MJPEG](#module-mjpeg)
- first project in the World with support streaming from [HomeKit Cameras](#source-homekit)
- first project in the World with support H265 for WebRTC in browser ([read more](https://github.com/AlexxIT/Blog/issues/5))
- on the fly transcoding for unsupported codecs via [FFmpeg](#source-ffmpeg)
- multi-source 2-way [codecs negotiation](#codecs-negotiation)
   - mixing tracks from different sources to single stream
   - auto match client supported codecs
   - 2-way audio for `ONVIF Profile T` Cameras
- streaming from private networks via [Ngrok](#module-ngrok)
- can be [integrated to](#module-api) any smart home platform or be used as [standalone app](#go2rtc-binary)

**Inspired by:**

- series of streaming projects from [@deepch](https://github.com/deepch)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [rtsp-simple-server](https://github.com/aler9/rtsp-simple-server) idea from [@aler9](https://github.com/aler9)
- [GStreamer](https://gstreamer.freedesktop.org/) framework pipeline idea
- [MediaSoup](https://mediasoup.org/) framework routing idea
- HomeKit Accessory Protocol from [@brutella](https://github.com/brutella/hap)

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

- `go2rtc_win64.zip` - Windows 64-bit
- `go2rtc_win32.zip` - Windows 32-bit
- `go2rtc_win_arm64.zip` - Windows ARM 64-bit
- `go2rtc_linux_amd64` - Linux 64-bit
- `go2rtc_linux_i386` - Linux 32-bit
- `go2rtc_linux_arm64` - Linux ARM 64-bit (ex. Raspberry 64-bit OS)
- `go2rtc_linux_arm` - Linux ARM 32-bit (ex. Raspberry 32-bit OS)
- `go2rtc_linux_mipsel` - Linux MIPS (ex. [Xiaomi Gateway 3](https://github.com/AlexxIT/XiaomiGateway3))
- `go2rtc_mac_amd64.zip` - Mac Intel 64-bit
- `go2rtc_mac_arm64.zip` - Mac ARM 64-bit

Don't forget to fix the rights `chmod +x go2rtc_xxx_xxx` on Linux and Mac.

### go2rtc: Home Assistant Add-on

[![](https://my.home-assistant.io/badges/supervisor_addon.svg)](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons)

1. Install Add-On:
    - Settings > Add-ons > Plus > Repositories > Add `https://github.com/AlexxIT/hassio-addons`
    - go2rtc > Install > Start
2. Setup [Integration](#module-hass)

### go2rtc: Docker

Container [alexxit/go2rtc](https://hub.docker.com/r/alexxit/go2rtc) with support `amd64`, `386`, `arm64`, `arm`. This container same as [Home Assistant Add-on](#go2rtc-home-assistant-add-on), but can be used separately from the Home Assistant. Container has preinstalled [FFmpeg](#source-ffmpeg), [Ngrok](#module-ngrok) and [Python](#source-echo).

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
- [mp4](#module-mp4) - MSE, MP4 stream and MP4 shapshot Server
- [mjpeg](#module-mjpeg) - MJPEG Server
- [ffmpeg](#source-ffmpeg) - FFmpeg integration
- [ngrok](#module-ngrok) - Ngrok integration (external access for private network)
- [hass](#module-hass) - Home Assistant integration
- [log](#module-log) - logs config

### Module: Streams

**go2rtc** support different stream source types. You can config one or multiple links of any type as stream source.

Available source types:

- [rtsp](#source-rtsp) - `RTSP` and `RTSPS` cameras
- [rtmp](#source-rtmp) - `RTMP` streams
- [http](#source-http) - `HTTP-FLV`, `JPEG` (snapshots), `MJPEG` streams 
- [ffmpeg](#source-ffmpeg) - FFmpeg integration (`HLS`, `files` and many others)
- [ffmpeg:device](#source-ffmpeg-device) - local USB Camera or Webcam
- [exec](#source-exec) - advanced FFmpeg and GStreamer integration
- [echo](#source-echo) - get stream link from bash or python
- [homekit](#source-homekit) - streaming from HomeKit Camera
- [ivideon](#source-ivideon) - public cameras from [Ivideon](https://tv.ivideon.com/) service
- [hass](#source-hass) - Home Assistant integration

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

**PS.** For disable bachannel just add `#backchannel=0` to end of RTSP link.

#### Source: RTMP

You can get stream from RTMP server, for example [Frigate](https://docs.frigate.video/configuration/rtmp).

```yaml
streams:
  rtmp_stream: rtmp://192.168.1.123/live/camera1
```

#### Source: HTTP

Support Content-Type:

- **HTTP-FLV** (`video/x-flv`) - same as RTMP, but over HTTP
- **HTTP-JPEG** (`image/jpeg`) - camera snapshot link, can be converted by go2rtc to MJPEG stream
- **HTTP-MJPEG** (`multipart/x`) - simple MJPEG stream over HTTP

```yaml
streams:
  # [HTTP-FLV] stream in video/x-flv format
  http_flv: http://192.168.1.123:20880/api/camera/stream/780900131155/657617
  
  # [JPEG] snapshots from Dahua camera, will be converted to MJPEG stream
  dahua_snap: http://admin:password@192.168.1.123/cgi-bin/snapshot.cgi?channel=1

  # [MJPEG] stream will be proxied without modification
  http_mjpeg: https://mjpeg.sanford.io/count.mjpeg
```

**PS.** Dahua camera has bug: if you select MJPEG codec for RTSP second stream - snapshot won't work.

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

All trascoding formats has [built-in templates](https://github.com/AlexxIT/go2rtc/blob/master/cmd/ffmpeg/ffmpeg.go): `h264`, `h264/ultra`, `h264/high`, `h265`, `opus`, `pcmu`, `pcmu/16000`, `pcmu/48000`, `pcma`, `pcma/16000`, `pcma/48000`, `aac`, `aac/16000`.

But you can override them via YAML config. You can also add your own formats to config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg                                        # path to ffmpeg binary
  h264: "-codec:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency -profile:v main -level:v 4.1"
  mycodec: "-any args that support ffmpeg..."
```

- You can use `video` and `audio` params multiple times (ex. `#video=copy#audio=copy#audio=pcmu`)
- You can use go2rtc stream name as ffmpeg input (ex. `ffmpeg:camera1#video=h264`)
- You can use `rotate` params with `90`, `180`, `270` or `-90` values, important with transcoding (ex. `#video=h264#rotate=90`)
- You can use `raw` param for any additional FFmpeg arguments (ex. `#raw=-vf transpose=1`).

#### Source: FFmpeg Device

You can get video from any USB-camera or Webcam as RTSP or WebRTC stream. This is part of FFmpeg integration.

- check available devices in Web interface
- `resolution` and `framerate` must be supported by your camera!
- for Linux supported only video for now
- for macOS you can stream Facetime camera or whole Desktop!
- for macOS important to set right framerate

```yaml
streams:
  linux_usbcam:   ffmpeg:device?video=0&resolution=1280x720#video=h264
  windows_webcam: ffmpeg:device?video=0#video=h264
  macos_facetime: ffmpeg:device?video=0&audio=1&resolution=1280x720&framerate=30#video=h264#audio=pcma
```

#### Source: Exec

FFmpeg source just a shortcut to exec source. You can get any stream or file or device via FFmpeg or GStreamer and push it to go2rtc via RTSP protocol: 

```yaml
streams:
  stream1: exec:ffmpeg -hide_banner -re -stream_loop -1 -i /media/BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp {output}
```

#### Source: Echo

Some sources may have a dynamic link. And you will need to get it using a bash or python script. Your script should echo a link to the source. RTSP, FFmpeg or any of the [supported sources](#module-streams).

**Docker** and **Hass Add-on** users has preinstalled `python3`, `curl`, `jq`.

Check examples in [wiki](https://github.com/AlexxIT/go2rtc/wiki/Source-Echo-examples).

```yaml
streams:
  apple_hls: echo:python3 hls.py https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html
```

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
- Audio can be transcoded by [ffmpeg](#source-ffmpeg) source with `#async` option
- Audio can be played by `ffplay` with `-use_wallclock_as_timestamps 1 -async 1` options
- Audio can't be played in `VLC` and probably any other player

Recommended settings for using HomeKit Camera with WebRTC, MSE, MP4, RTSP:

```
streams:
  aqara_g3:
    - hass:Camera-Hub-G3-AB12
    - ffmpeg:aqara_g3#audio=aac#audio=opus#async
```

RTSP link with "normal" audio for any player: `rtsp://192.168.1.123:8554/aqara_g3?video&audio=aac`

**This source is in active development!** Tested only with [Aqara Camera Hub G3](https://www.aqara.com/eu/product/camera-hub-g3) (both EU and CN versions).

#### Source: Ivideon

Support public cameras from service [Ivideon](https://tv.ivideon.com/).

```yaml
streams:
  quailcam: ivideon:100-tu5dkUPct39cTp9oNEN2B6/0
```

#### Source: Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- support [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI
- support [HomeKit Camera](https://www.home-assistant.io/integrations/homekit_controller/)

```yaml
hass:
  config: "/config"  # skip this setting if you Hass Add-on user

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
  aqara_g3: hass:Camera-Hub-G3-AB12
```

More cameras, like [Tuya](https://www.home-assistant.io/integrations/tuya/), [ONVIF](https://www.home-assistant.io/integrations/onvif/), and possibly others can also be imported by using [this method](https://github.com/felipecrs/hass-expose-camera-stream-source#importing-home-assistant-cameras-to-go2rtc-andor-frigate).

### Module: API

The HTTP API is the main part for interacting with the application. Default address: `http://127.0.0.1:1984/`.

go2rtc has its own JS video player (`video-rtc.js`) with:

- support technologies:
   - WebRTC over UDP or TCP
   - MSE or MP4 or MJPEG over WebSocket 
- automatic selection best technology according on:
   - codecs inside your stream
   - current browser capabilities
   - current network configuration
- automatic stop stream while browser or page not active
- automatic stop stream while player not inside page viewport
- automatic reconnection

Technology selection based on priorities:

1. Video and Audio better than just Video
2. H265 better than H264
3. WebRTC better than MSE, than MP4, than MJPEG

go2rtc has simple HTML page (`stream.html`) with support params in URL:

- multiple streams on page `src=camera1&src=camera2...`
- stream technology autoselection `mode=webrtc,mse,mp4,mjpeg`
- stream technology comparison `src=camera1&mode=webrtc&mode=mse&mode=mp4`
- player width setting in pixels `width=320px` or percents `width=50%`

**Module config**

- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "127.0.0.1:1984"` setting
- you can change API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`

```yaml
api:
  listen: ":1984"    # default ":1984", HTTP API port ("" - disabled)
  base_path: "/rtc"  # default "", API prefix for serve on suburl (/api => /rtc/api)
  static_dir: "www"  # default "", folder for static files (custom web interface)
  origin: "*"        # default "", allow CORS requests (only * supported)
```

**PS. go2rtc** doesn't provide HTTPS or password protection. Use [Nginx](https://nginx.org/) or [Ngrok](#module-ngrok) or [Home Assistant Add-on](#go2rtc-home-assistant-add-on) for this tasks.

**PS2.** You can access microphone (for 2-way audio) only with HTTPS ([read more](https://stackoverflow.com/questions/52759992/how-to-access-camera-and-microphone-in-chrome-without-https)).

**PS3.** MJPEG over WebSocket plays better than native MJPEG because Chrome [bug](https://bugs.chromium.org/p/chromium/issues/detail?id=527446).

**PS4.** MP4 over WebSocket was created only for Apple iOS because it doesn't support MSE and native MP4.

### Module: RTSP

You can get any stream as RTSP-stream: `rtsp://192.168.1.123:8554/{stream_name}`

- you can omit the codec filters, so one first video and one first audio will be selected
- you can set `?video=copy` or just `?video`, so only one first video without audio will be selected
- you can set multiple video or audio, so all of them will be selected
- you can enable external password protection for your RTSP streams

Password protection always disabled for localhost calls (ex. FFmpeg or Hass on same server)

```yaml
rtsp:
  listen: ":8554"  # RTSP Server TCP port, default - 8554
  username: admin  # optional, default - disabled
  password: pass   # optional, default - disabled
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

If you install **go2rtc** as [Hass Add-on](#go2rtc-home-assistant-add-on) - you need to use localhost IP-address. In other cases you need to use IP-address of server with **go2rtc** application.

#### From go2rtc to Hass

Add any supported [stream source](#module-streams) as [Generic Camera](https://www.home-assistant.io/integrations/generic/) and view stream with built-in [Stream](https://www.home-assistant.io/integrations/stream/) integration. Technology `HLS`, supported codecs: `H264`, poor latency.

1. Add your stream to [go2rtc config](#configuration)
2. Hass > Settings > Integrations > Add Integration > [Generic Camera](https://my.home-assistant.io/redirect/config_flow_start/?domain=generic) > `rtsp://127.0.0.1:8554/camera1`

#### From Hass to go2rtc

View almost any Hass camera using `WebRTC` technology, supported codecs `H264`/`PCMU`/`PCMA`/`OPUS`, best latency.

When the stream starts - the camera `entity_id` will be added to go2rtc "on the fly". You don't need to add cameras manually to [go2rtc config](#configuration). Some cameras (like [Nest](https://www.home-assistant.io/integrations/nest/)) have a dynamic link to the stream, it will be updated each time a stream is started from the Hass interface. 

1. Hass > Settings > Integrations > Add Integration > [RTSPtoWebRTC](https://my.home-assistant.io/redirect/config_flow_start/?domain=rtsp_to_webrtc) > `http://127.0.0.1:1984/`
2. RTSPtoWebRTC > Configure > STUN server: `stun.l.google.com:19302`
3. Use Picture Entity or Picture Glance lovelace card

You can add camera `entity_id` to [go2rtc config](#configuration) if you need transcoding:

```yaml
streams:
  "camera.hall": ffmpeg:{input}#video=copy#audio=opus
```

PS. Default Home Assistant lovelace cards don't support 2-way audio. You can use 2-way audio from [Add-on Web UI](https://my.home-assistant.io/redirect/supervisor_addon/?addon=a889bffc_go2rtc&repository_url=https%3A%2F%2Fgithub.com%2FAlexxIT%2Fhassio-addons). But you need use HTTPS to access the microphone. This is a browser restriction and cannot be avoided.

### Module: MP4

Provides several features:

1. MSE stream (fMP4 over WebSocket)
2. Camera snapshots in MP4 format (single frame), can be sent to [Telegram](https://www.telegram.org/)
3. MP4 "file stream" - bad format for streaming because of high latency, doesn't work in Safari 

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

## Codecs madness

`AVC/H.264` codec can be played almost anywhere. But `HEVC/H.265` has a lot of limitations in supporting with different devices and browsers. It's all about patents and money, you can't do anything about it.

| Device              | WebRTC      | MSE         | MP4         |
|---------------------|-------------|-------------|-------------|
| *latency*           | best        | medium      | bad         |
| Desktop Chrome 107+ | H264        | H264, H265* | H264, H265* |
| Desktop Edge        | H264        | H264, H265* | H264, H265* |
| Desktop Safari      | H264, H265* | H264, H265  | **no!**     |
| Desktop Firefox     | H264        | H264        | H264        |
| Android Chrome 107+ | H264        | H264, H265* | H264        |
| iPad Safari 13+     | H264, H265* | H264, H265  | **no!**     |
| iPhone Safari 13+   | H264, H265* | **no!**     | **no!**     |
| masOS Hass App      | no          | no          | no          |

- Chrome H265: [read this](https://chromestatus.com/feature/5186511939567616) and [read this](https://github.com/StaZhu/enable-chromium-hevc-hardware-decoding)
- Edge H265: [read this](https://www.reddit.com/r/MicrosoftEdge/comments/v9iw8k/enable_hevc_support_in_edge/)
- Desktop Safari H265: Menu > Develop > Experimental > WebRTC H265
- iOS Safari H265: Settings > Safari > Advanced > Experimental > WebRTC H265

**Audio**

- WebRTC audio codecs: `PCMU/8000`, `PCMA/8000`, `OPUS/48000/2`
- MSE/MP4 audio codecs: `AAC`

**Apple devices**

- all Apple devices don't support MP4 stream (they only support progressive loading of static files)
- iPhones don't support MSE technology because it competes with the HLS technology, invented by Apple
- HLS is the worst technology for **live** streaming, it still exists only because of iPhones

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

## TIPS

**Using apps for low RTSP delay**

- `ffplay -fflags nobuffer -flags low_delay "rtsp://192.168.1.123:8554/camera1"`
- VLC > Preferences > Input / Codecs > Default Caching Level: Lowest Latency

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
