# WebRTC

## WebRTC Client

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This source type supports four connection formats.

### Creality

[`new in v1.9.10`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.10)

[Creality](https://www.creality.com/) 3D printer camera. Read more [here](https://github.com/AlexxIT/go2rtc/issues/1600).

```yaml
streams:
  creality_k2p: webrtc:http://192.168.1.123:8000/call/webrtc_local#format=creality
```

### go2rtc

This format is only supported in go2rtc. Unlike WHEP, it supports asynchronous WebRTC connections and two-way audio.

```yaml
streams:
  webrtc-go2rtc:    webrtc:ws://192.168.1.123:1984/api/ws?src=camera1
```

### Kinesis

[`new in v1.6.1`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)

Supports [Amazon Kinesis Video Streams](https://aws.amazon.com/kinesis/video-streams/), using WebRTC protocol. You need to specify the signaling WebSocket URL with all credentials in query params, `client_id` and `ice_servers` list in [JSON format](https://developer.mozilla.org/en-US/docs/Web/API/RTCIceServer).

```yaml
streams:
  webrtc-kinesis:   webrtc:wss://...amazonaws.com/?...#format=kinesis#client_id=...#ice_servers=[{...},{...}]
```

**PS.** For `kinesis` sources, you can use [echo](../echo/README.md) to get connection params using `bash`, `python` or any other script language.

### OpenIPC

[`new in v1.7.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)

Cameras on open-source [OpenIPC](https://openipc.org/) firmware.

```yaml
streams:
  webrtc-openipc:   webrtc:ws://192.168.1.123/webrtc_ws#format=openipc#ice_servers=[{"urls":"stun:stun.kinesisvideo.eu-north-1.amazonaws.com:443"}]
```

### SwitchBot

Support connection to [SwitchBot](https://us.switch-bot.com/) cameras that are based on Kinesis Video Streams. Specifically, this includes [Pan/Tilt Cam Plus 2K](https://us.switch-bot.com/pages/switchbot-pan-tilt-cam-plus-2k) and [Pan/Tilt Cam Plus 3K](https://us.switch-bot.com/pages/switchbot-pan-tilt-cam-plus-3k) and [Smart Video Doorbell](https://www.switchbot.jp/products/switchbot-smart-video-doorbell). `Outdoor Spotlight Cam 1080P`, `Outdoor Spotlight Cam 2K`, `Pan/Tilt Cam`, `Pan/Tilt Cam 2K`, `Indoor Cam` are based on Tuya, so this feature is not available.

```yaml
streams:
  webrtc-switchbot: webrtc:wss://...amazonaws.com/?...#format=switchbot#resolution=hd#play_type=0#client_id=...#ice_servers=[{...},{...}]
```

### WHEP

[WebRTC/WHEP](https://datatracker.ietf.org/doc/draft-murillo-whep/) is replaced by [WebRTC/WISH](https://datatracker.ietf.org/doc/charter-ietf-wish/02/) standard for WebRTC video/audio viewers. But it may already be supported in some third-party software. It is supported in go2rtc.

```yaml
streams:
  webrtc-whep:      webrtc:http://192.168.1.123:1984/api/webrtc?src=camera1
```

### Wyze

[`new in v1.6.1`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)

Legacy method to connect to [Wyze](https://www.wyze.com/) cameras using WebRTC protocol via [docker-wyze-bridge](https://github.com/mrlt8/docker-wyze-bridge). For native P2P support without docker-wyze-bridge, see [Source: Wyze](../wyze/README.md).

```yaml
streams:
  webrtc-wyze:      webrtc:http://192.168.1.123:5000/signaling/camera1?kvs#format=wyze
```

## WebRTC Server

What you should know about WebRTC:

- It's almost always a **direct [peer-to-peer](https://en.wikipedia.org/wiki/Peer-to-peer) connection** from your browser to the go2rtc app
- When you use Home Assistant, Frigate, Nginx, Nabu Casa, Cloudflare, and other software, they are only **involved in establishing** the connection; they are **not involved in transferring** media data
- WebRTC media cannot be transferred inside an HTTP connection
- Usually, WebRTC uses random UDP ports on the client and server to establish a connection
- Usually, WebRTC uses public [STUN](https://en.wikipedia.org/wiki/STUN) servers to establish a connection outside the LAN; these servers are only needed to establish a connection and are not involved in data transfer
- Usually, WebRTC will automatically discover all of your local and public addresses and try to establish a connection

If an external connection via STUN is used:

- Uses [UDP hole punching](https://en.wikipedia.org/wiki/UDP_hole_punching) technology to bypass NAT even if you haven't opened your server to the world
- For about 20% of users, the technology will not work because of the [Symmetric NAT](https://tomchen.github.io/symmetric-nat-test/)
- UDP is not suitable for transmitting 2K and 4K high bit rate video over open networks because of the high loss rate:
  - https://habr.com/ru/companies/flashphoner/articles/480006/
  - https://www.youtube.com/watch?v=FXVg2ckuKfs

### Configuration suggestions

- by default, WebRTC uses both TCP and UDP on port 8555 for connections
- you can use this port for external access
- you can change the port in YAML config:

```yaml
webrtc:
  listen: ":8555"  # address of your local server and port (TCP/UDP)
```

#### Static public IP

- forward the port 8555 on your router (you can use the same 8555 port or any other as external port)
- add your external IP address and external port to the YAML config

```yaml
webrtc:
  candidates:
    - 216.58.210.174:8555  # if you have a static public IP address
```

#### Dynamic public IP

- forward the port 8555 on your router (you can use the same 8555 port or any other as the external port)
- add `stun` word and external port to YAML config
    - go2rtc automatically detects your external address with STUN server

```yaml
webrtc:
  candidates:
    - stun:8555  # if you have a dynamic public IP address
```

#### Hard tech way 1. Own TCP-tunnel

If you have a personal [VPS](https://en.wikipedia.org/wiki/Virtual_private_server), you can create a TCP tunnel and setup in the same way as "Static public IP". But use your VPS IP address in the YAML config.

#### Hard tech way 2. Using TURN-server

If you have personal [VPS](https://en.wikipedia.org/wiki/Virtual_private_server), you can install TURN server (e.g. [coturn](https://github.com/coturn/coturn), config [example](https://github.com/AlexxIT/WebRTC/wiki/Coturn-Example)).

```yaml
webrtc:
  ice_servers:
    - urls: [stun:stun.l.google.com:19302]
    - urls: [turn:123.123.123.123:3478]
      username: your_user
      credential: your_pass
```

### Full configuration

**Important!** This example is not for copy/pasting!

```yaml
webrtc:
  # fix local TCP or UDP or both ports for WebRTC media
  listen: ":8555"            # address of your local server

  # add additional host candidates manually
  # order is important, the first will have a higher priority
  candidates:
    - 216.58.210.174:8555    # if you have static public IP-address
    - stun:8555              # if you have dynamic public IP-address
    - home.duckdns.org:8555  # if you have domain

  # add custom STUN and TURN servers
  # use `ice_servers: []` to remove defaults and leave it empty
  ice_servers:
    - urls: [ stun:stun1.l.google.com:19302 ]
    - urls: [ turn:123.123.123.123:3478 ]
      username: your_user
      credential: your_pass

  # optional filter list for auto-discovery logic
  # some settings only make sense if you don't specify a fixed UDP port
  filters:
    # list of host candidates from auto-discovery to be sent
    # includes candidates from the `listen` option
    # use `candidates: []` to remove all auto-discovery candidates
    candidates: [ 192.168.1.123 ]

    # enable localhost candidates
    loopback: true

    # list of network types to be used for the connection
    # includes candidates from the `listen` option
    networks: [ udp4, udp6, tcp4, tcp6 ]

    # list of interfaces to be used for the connection
    # includes interfaces from unspecified `listen` option (empty host)
    interfaces: [ eno1 ]

    # list of host IP addresses to be used for the connection
    # includes IPs from unspecified `listen` option (empty host)
    ips: [ 192.168.1.123 ]

    # range for random UDP ports [min, max] to be used for connection
    # not related to the `listen` option
    udp_ports: [ 50000, 50100 ]
```

By default, go2rtc uses a **fixed TCP** port and **fixed UDP** ports for each **direct** WebRTC connection: `listen: ":8555"`.

You can set a **fixed TCP** port and a **random UDP** port for all connections: `listen: ":8555/tcp"`.

You can also disable the TCP port and leave only random UDP ports: `listen: ""`.

### Configuration filters

**Important!** By default, go2rtc excludes all Docker-like candidates (`172.16.0.0/12`). This cannot be disabled.

Filters allow you to exclude unnecessary candidates. Extra candidates don't make your connection worse or better. But the wrong filter settings can break everything. Skip this setting if you don't understand it.

For example, go2rtc is installed on the host system. And there are unnecessary interfaces. You can keep only the relevant via `interfaces` or `ips` options. You can also exclude IPv6 candidates if your server supports them but your home network does not.

```yaml
webrtc:
  listen: ":8555/tcp"         # use fixed TCP port and random UDP ports
  filters:
    ips: [ 192.168.1.2 ]      # IP-address of your server
    networks: [ udp4, tcp4 ]  # skip IPv6, if it's not supported for you
```

For example, go2rtc is inside a closed Docker container (e.g. [Frigate](https://frigate.video/)). You shouldn't filter Docker interfaces; otherwise, go2rtc won't be able to connect anywhere. But you can filter the Docker candidates because no one can connect to them.

```yaml
webrtc:
  listen: ":8555"                   # use fixed TCP and UDP ports
  candidates: [ 192.168.1.2:8555 ]  # add manual host candidate (use docker port forwarding)
```

## Streaming ingest

### Ingest: Browser

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

You can turn the browser of any PC or mobile into an IP camera with support for video and two-way audio. Or even broadcast your PC screen:

1. Create empty stream in the `go2rtc.yaml`
2. Go to go2rtc WebUI
3. Open `links` page for your stream
4. Select `camera+microphone` or `display+speaker` option
5. Open `webrtc` local page (your go2rtc **should work over HTTPS!**) or `share link` via [WebTorrent](../webtorrent/README.md) technology (work over HTTPS by default)

### Ingest: WHIP

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

You can use **OBS Studio** or any other broadcast software with [WHIP](https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html) protocol support. This standard has not yet been approved. But you can download OBS Studio [dev version](https://github.com/obsproject/obs-studio/actions/runs/3969201209):

- Settings > Stream > Service: WHIP > `http://192.168.1.123:1984/api/webrtc?dst=camera1`

## Useful links

- https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html
- https://www.ietf.org/id/draft-murillo-whep-01.html
- https://github.com/Glimesh/broadcast-box/
- https://github.com/obsproject/obs-studio/pull/7926
- https://misi.github.io/webrtc-c0d3l4b/
- https://github.com/webtorrent/webtorrent/blob/master/docs/faq.md
