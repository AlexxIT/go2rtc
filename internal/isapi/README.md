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

## Codecs

| Camera TwoWayAudio | Support |
|--------------------|---------|
| G.711ulaw / G.711alaw | Raw PCMU/PCMA (classic path) |
| AAC | Length-prefixed ADTS over `/audioData` (`[u32be len][ADTS]`). WebRTC mic (PCMU/PCMA) is transcoded to AAC via `ffmpeg` |

Requires `ffmpeg` on PATH when the camera is set to AAC and the browser sends G.711.

## Notes

- Session: `close` → brief settle → `open` → `PUT .../audioData?sessionId=...`
- AAC sample rate is read from `audioSamplingRate` (typically 16 kHz)
- Some firmware lists G.711 in capabilities but rejects `open` while AAC works — leave the camera on AAC
