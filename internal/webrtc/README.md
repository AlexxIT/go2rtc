What you should to know about WebRTC:

- It's almost always a **direct [peer-to-peer](https://en.wikipedia.org/wiki/Peer-to-peer) connection** from your browser to go2rtc app
- When you use Home Assistant, Frigate, Nginx, Nabu Casa, Cloudflare and other software - they are only **involved in establishing** the connection, but they are **not involved in transferring** media data
- WebRTC media cannot be transferred inside an HTTP connection
- Usually, WebRTC uses random UDP ports on client and server side to establish a connection
- Usually, WebRTC uses public [STUN](https://en.wikipedia.org/wiki/STUN) servers to establish a connection outside LAN, such servers are only needed to establish a connection and are not involved in data transfer
- Usually, WebRTC will automatically discover all of your local addresses and all of your public addresses and try to establish a connection

If an external connection via STUN is used:

- Uses [UDP hole punching](https://en.wikipedia.org/wiki/UDP_hole_punching) technology to bypass NAT even if you not open your server to the World
- For about 20% of users, the techology will not work because of the [Symmetric NAT](https://tomchen.github.io/symmetric-nat-test/)
- UDP is not suitable for transmitting 2K and 4K high bitrate video over open networks because of the high loss rate

## Default config

```yaml
webrtc:
  listen: ":8555/tcp"
  ice_servers:
    - urls: [ "stun:stun.l.google.com:19302" ]
```

## Config

**Important!** This example is not for copypasting!

```yaml
webrtc:
  # fix local TCP or UDP or both ports for WebRTC media
  listen: ":8555/tcp"        # address of your local server

  # add additional host candidates manually
  # order is important, the first will have a higher priority
  candidates:
    - 216.58.210.174:8555    # if you have static public IP-address
    - stun:8555              # if you have dynamic public IP-address
    - home.duckdns.org:8555  # if you have domain

  # add custom STUN and TURN servers
  # use `ice_servers: []` for remove defaults and leave empty
  ice_servers:
    - urls: [ stun:stun1.l.google.com:19302 ]
    - urls: [ turn:123.123.123.123:3478 ]
      username: your_user
      credential: your_pass

  # optional filter list for auto discovery logic
  # some settings only make sense if you don't specify a fixed UDP port  
  filters:
    # list of host candidates from auto discovery to be sent
    # including candidates from the `listen` option
    # use `candidates: []` to remove all auto discovery candidates
    candidates: [ 192.168.1.123 ]

    # list of network types to be used for connection
    # including candidates from the `listen` option
    networks: [ udp4, udp6, tcp4, tcp6 ]

    # list of interfaces to be used for connection
    # not related to the `listen` option
    interfaces: [ eno1 ]

    # list of host IP-addresses to be used for connection
    # not related to the `listen` option
    ips: [ 192.168.1.123 ]

    # range for random UDP ports [min, max] to be used for connection
    # not related to the `listen` option
    udp_ports: [ 50000, 50100 ]
```

By default go2rtc uses **fixed TCP** port and multiple **random UDP** ports for each WebRTC connection - `listen: ":8555/tcp"`.

You can set **fixed TCP** and **fixed UDP** port for all connections - `listen: ":8555"`. This may has lower performance, but it's your choice. 

Don't know why, but you can disable TCP port and leave only random UDP ports - `listen: ""`.

## Config filters

Filters allow you to exclude unnecessary candidates. Extra candidates don't make your connection worse or better. But the wrong filter settings can break everything. Skip this setting if you don't understand it.

For example, go2rtc is installed on the host system. And there are unnecessary interfaces. You can keep only the relevant via `interfaces` or `ips` options. You can also exclude IPv6 candidates if your server supports them but your home network does not.

```yaml
webrtc:
  listen: ":8555/tcp"         # use fixed TCP port and random UDP ports
  filters:
    ips: [ 192.168.1.2 ]      # IP-address of your server
    networks: [ udp4, tcp4 ]  # skip IPv6, if it's not supported for you
```

For example, go2rtc inside closed docker container (ex. [Frigate](https://frigate.video/)). You shouldn't filter docker interfaces, otherwise go2rtc will not be able to connect anywhere. But you can filter the docker candidates because no one can connect to them.

```yaml
webrtc:
  listen: ":8555"                   # use fixed TCP and UDP ports
  candidates: [ 192.168.1.2:8555 ]  # add manual host candidate (use docker port forwarding)
  filters:
    candidates: []                  # skip all internal docker candidates
```

## Userful links

- https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html
- https://www.ietf.org/id/draft-murillo-whep-01.html
- https://github.com/Glimesh/broadcast-box/
- https://github.com/obsproject/obs-studio/pull/7926
- https://misi.github.io/webrtc-c0d3l4b/
- https://github.com/webtorrent/webtorrent/blob/master/docs/faq.md
