## Config

- supported TCP: fixed port (default), disabled 
- supported UDP: random port (default), fixed port

| Config examples       | TCP   | UDP    |
|-----------------------|-------|--------|
| `listen: ":8555/tcp"` | fixed | random |
| `listen: ":8555"`     | fixed | fixed  |
| `listen: ""`          | no    | random |

## Userful links

- https://www.ietf.org/archive/id/draft-ietf-wish-whip-01.html
- https://www.ietf.org/id/draft-murillo-whep-01.html
- https://github.com/Glimesh/broadcast-box/
- https://github.com/obsproject/obs-studio/pull/7926
- https://misi.github.io/webrtc-c0d3l4b/
- https://github.com/webtorrent/webtorrent/blob/master/docs/faq.md
