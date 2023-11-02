## Config

- supported networks: IPv4 (default), IPv6, both
- supported TCP: fixed port (default), disabled 
- supported UDP: random port (default), fixed port

| Config examples              | IPv4 | IPv6 | TCP   | UDP    |
|------------------------------|------|------|-------|--------|
| `listen: "0.0.0.0:8555/tcp"` | yes  | no   | fixed | random |
| `listen: "0.0.0.0:8555/udp"` | yes  | no   | no    | fixed  |
| `listen: "[::]:8555/tcp"`    | no   | yes  | fixed | random |
| `listen: ":8555"`            | yes  | yes  | fixed | fixed  |
| `listen: ""`                 | yes  | yes  | no    | random |

## Userful links

- https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html
- https://www.ietf.org/id/draft-murillo-whep-01.html
- https://github.com/Glimesh/broadcast-box/
- https://github.com/obsproject/obs-studio/pull/7926
- https://misi.github.io/webrtc-c0d3l4b/
- https://github.com/webtorrent/webtorrent/blob/master/docs/faq.md
