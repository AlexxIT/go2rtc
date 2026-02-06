# Doorbird

[`new in v1.9.8`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.8)

This source type supports [Doorbird](https://www.doorbird.com/) devices including MJPEG stream, audio stream as well as two-way audio.

It is recommended to create a sepearate user within your doorbird setup for go2rtc. Minimum permissions for the user are:

- Watch always
- API operator

## Configuration

```yaml
streams:
  doorbird1:
    - rtsp://admin:password@192.168.1.123:8557/mpeg/720p/media.amp  # RTSP stream
    - doorbird://admin:password@192.168.1.123?media=video           # MJPEG stream
    - doorbird://admin:password@192.168.1.123?media=audio           # audio stream
    - doorbird://admin:password@192.168.1.123                       # two-way audio
```
