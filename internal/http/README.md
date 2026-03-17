# HTTP

This source supports receiving a stream via an HTTP link.

It can determine the source format from the`Content-Type` HTTP header:

- **HTTP-JPEG** (`image/jpeg`) - camera snapshot link, can be converted by go2rtc to MJPEG stream
- **HTTP-MJPEG** (`multipart/x-mixed-replace`) - A continuous sequence of JPEG frames (with HTTP headers).
- **HLS** (`application/vnd.apple.mpegurl`) - A popular [HTTP Live Streaming](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) (HLS) format, which is not designed for real-time media transmission.

> [!WARNING]
> The HLS format is not designed for real time and is supported quite poorly. It is recommended to use it via ffmpeg source with buffering enabled (disabled by default).

## TCP

Source also supports HTTP and TCP streams with autodetection for different formats:

- `adts` - Audio stream in [AAC](https://en.wikipedia.org/wiki/Advanced_Audio_Coding) codec with Audio Data Transport Stream (ADTS) headers.
- `flv` - The legacy but still used [Flash Video](https://en.wikipedia.org/wiki/Flash_Video) format.
- `h264` - AVC/H.264 bitstream.
- `hevc` - HEVC/H.265 bitstream.
- `mjpeg` - A continuous sequence of JPEG frames (without HTTP headers).
- `mpegts` - The legacy [MPEG transport stream](https://en.wikipedia.org/wiki/MPEG_transport_stream) format.
- `wav` - Audio stream in [WAV](https://en.wikipedia.org/wiki/WAV) format.
- `yuv4mpegpipe` - Raw YUV frame stream with YUV4MPEG header.

## Configuration

```yaml
streams:
  # [HTTP-FLV] stream in video/x-flv format
  http_flv: http://192.168.1.123:20880/api/camera/stream/780900131155/657617
  
  # [JPEG] snapshots from Dahua camera, will be converted to MJPEG stream
  dahua_snap: http://admin:password@192.168.1.123/cgi-bin/snapshot.cgi?channel=1

  # [MJPEG] stream will be proxied without modification
  http_mjpeg: https://mjpeg.sanford.io/count.mjpeg

  # [MJPEG or H.264/H.265 bitstream or MPEG-TS]
  tcp_magic: tcp://192.168.1.123:12345

  # Add custom header
  custom_header: "https://mjpeg.sanford.io/count.mjpeg#header=Authorization: Bearer XXX"
```

**PS.** Dahua camera has a bug: if you select MJPEG codec for RTSP second stream, snapshot won't work.
