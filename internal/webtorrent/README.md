# WebTorrent

> [!NOTE]
> This section needs some improvement.

## WebTorrent Client

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This source can get a stream from another go2rtc via [WebTorrent](https://en.wikipedia.org/wiki/WebTorrent) protocol.

### Client Configuration

```yaml
streams:
  webtorrent1: webtorrent:?share=huofssuxaty00izc&pwd=k3l2j9djeg8v8r7e
```

## WebTorrent Server

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This module supports:

- Share any local stream via [WebTorrent](https://webtorrent.io/) technology
- Get any [incoming stream](../webrtc/README.md#ingest-browser) from PC or mobile via [WebTorrent](https://webtorrent.io/) technology
- Get any remote go2rtc source via [WebTorrent](https://webtorrent.io/) technology

Securely and freely. You do not need to open a public access to the go2rtc server. But in some cases (Symmetric NAT), you may need to set up external access to [WebRTC module](../webrtc/README.md).

To generate a sharing link or incoming link, go to the go2rtc WebUI (stream links page). This link is **temporary** and will stop working after go2rtc is restarted!

### Server Configuration

You can create permanent external links in the go2rtc config:

```yaml
webtorrent:
  shares:
    super-secret-share:  # share name, should be unique among all go2rtc users!
      pwd: super-secret-password
      src: rtsp-dahua1   # stream name from streams section
```

Link example: `https://go2rtc.org/webtorrent/#share=02SNtgjKXY&pwd=wznEQqznxW&media=video+audio`
