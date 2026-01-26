# MJPEG

**Important.** For a stream in MJPEG format, your source MUST contain the MJPEG codec. If your stream has the MJPEG codec, you can receive an **MJPEG stream** or **JPEG snapshots** via the API.

You can receive an MJPEG stream in several ways:

- some cameras support MJPEG codec inside [RTSP stream](#source-rtsp) (ex. second stream for Dahua cameras)
- some cameras have an HTTP link with [MJPEG stream](#source-http)
- some cameras have an HTTP link with snapshots - go2rtc can convert them to [MJPEG stream](#source-http)
- you can convert an H264/H265 stream from your camera via [FFmpeg integration](#source-ffmpeg)

With this example, your stream will have both H264 and MJPEG codecs:

```yaml
streams:
  camera1:
    - rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
    - ffmpeg:camera1#video=mjpeg
```

## API examples

**MJPEG stream**

```
http://192.168.1.123:1984/api/stream.mjpeg?src=camera1
```

**JPEG snapshots**

```
http://192.168.1.123:1984/api/frame.jpeg?src=camera1
```

- You can use `width`/`w` and/or `height`/`h` parameters.
- You can use `rotate` param with `90`, `180`, `270` or `-90` values.
- You can use `hardware`/`hw` param [read more](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration).
- You can use `cache` param (`1m`, `10s`, etc.) to get a cached snapshot.
  - The snapshot is cached only when requested with the `cache` parameter.
  - A cached snapshot will be used if its time is not older than the time specified in the `cache` parameter.
  - The `cache` parameter does not check the image dimensions from the cache and those specified in the query.

## Stream as ASCII to Terminal

[![](https://img.youtube.com/vi/sHj_3h_sX7M/mqdefault.jpg)](https://www.youtube.com/watch?v=sHj_3h_sX7M)

**Tips**

- this feature works only with MJPEG codec (use transcoding)
- choose a low frame rate (FPS)
- choose the width and height to fit in your terminal
- different terminals support different numbers of colors (8, 256, rgb)
- URL-encode the `text` parameter
- you can stream any camera or file from disk

**go2rtc.yaml** - transcoding to MJPEG, terminal size - 210x59 (16/9), fps - 10

```yaml
streams:
  gamazda: ffmpeg:gamazda.mp4#video=mjpeg#hardware#width=210#height=59#raw=-r 10
```

**API params**

- `color` - foreground color, values: empty, `8`, `256`, `rgb`, [SGR](https://en.wikipedia.org/wiki/ANSI_escape_code)
  - example: `30` (black), `37` (white), `38;5;226` (yellow)
- `back` - background color, values: empty, `8`, `256`, `rgb`, [SGR](https://en.wikipedia.org/wiki/ANSI_escape_code)
  - example: `40` (black), `47` (white), `48;5;226` (yellow)
- `text` - character set, values: empty, one character, `block`, list of chars (in order of brightness)
  - example: `%20` (space), `block` (keyword for block elements), `ox` (two chars)

**Examples**

```bash
% curl "http://192.168.1.123:1984/api/stream.ascii?src=gamazda"
% curl "http://192.168.1.123:1984/api/stream.ascii?src=gamazda&color=256"
% curl "http://192.168.1.123:1984/api/stream.ascii?src=gamazda&back=256&text=%20"
% curl "http://192.168.1.123:1984/api/stream.ascii?src=gamazda&back=8&text=%20%20"
% curl "http://192.168.1.123:1984/api/stream.ascii?src=gamazda&text=helloworld"
```
