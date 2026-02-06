# Hikvision ISAPI

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This source type supports only backchannel audio for the [Hikvision ISAPI](https://tpp.hikvision.com/download/ISAPI_OTAP) protocol. So it should be used as a second source in addition to the RTSP protocol.

## Configuration

```yaml
streams:
  hikvision1:
    - rtsp://admin:password@192.168.1.123:554/Streaming/Channels/101
    - isapi://admin:password@192.168.1.123:80/
```
