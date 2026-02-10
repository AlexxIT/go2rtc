# Motion JPEG

- This module can provide and receive streams in MJPEG format.
- This module is also responsible for receiving snapshots in JPEG format.
- This module also supports streaming to the server console (terminal) in the **animated ASCII art** format.

## MJPEG Client

**Important.** For a stream in MJPEG format, your source MUST contain the MJPEG codec. If your stream has the MJPEG codec, you can receive an **MJPEG stream** or **JPEG snapshots** via the API.

You can receive an MJPEG stream in several ways:

- some cameras support MJPEG codec inside [RTSP stream](../rtsp/README.md) (ex. second stream for Dahua cameras)
- some cameras have an HTTP link with [MJPEG stream](../http/README.md)
- some cameras have an HTTP link with snapshots - go2rtc can convert them to [MJPEG stream](../http/README.md)
- you can convert an H264/H265 stream from your camera via [FFmpeg integration](../ffmpeg/README.md)

With this example, your stream will have both H264 and MJPEG codecs:

```yaml
streams:
  camera1:
    - rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
    - ffmpeg:camera1#video=mjpeg
```

## MJPEG Server

### mpjpeg

Output a stream in [MJPEG](https://en.wikipedia.org/wiki/Motion_JPEG) format. In [FFmpeg](https://ffmpeg.org/), this format is called `mpjpeg` because it contains HTTP headers.

```
ffplay http://192.168.1.123:1984/api/stream.mjpeg?src=camera1
```

### jpeg

Receiving a JPEG snapshot.

```
curl http://192.168.1.123:1984/api/frame.jpeg?src=camera1
```

- You can use `width`/`w` and/or `height`/`h` parameters.
- You can use `rotate` param with `90`, `180`, `270` or `-90` values.
- You can use `hardware`/`hw` param [read more](https://github.com/AlexxIT/go2rtc/wiki/Hardware-acceleration).
- You can use `cache` param (`1m`, `10s`, etc.) to get a cached snapshot.
  - The snapshot is cached only when requested with the `cache` parameter.
  - A cached snapshot will be used if its time is not older than the time specified in the `cache` parameter.
  - The `cache` parameter does not check the image dimensions from the cache and those specified in the query.

### ascii

Stream as ASCII to Terminal. This format is just for fun. You can boast to your friends that you can stream cameras even to the server console without a GUI.

[![](https://img.youtube.com/vi/sHj_3h_sX7M/mqdefault.jpg)](https://www.youtube.com/watch?v=sHj_3h_sX7M)

> The demo video features a combination of several settings for this format with added audio. Of course, the format doesn't support audio out of the box.

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

### yuv4mpegpipe

Raw [YUV](https://en.wikipedia.org/wiki/Y%E2%80%B2UV) frame stream with [YUV4MPEG](https://manned.org/yuv4mpeg) header.

```
ffplay http://192.168.1.123:1984/api/stream.y4m?src=camera1
```

## Streaming ingest

```shell
ffmpeg -re -i BigBuckBunny.mp4 -c mjpeg -f mpjpeg http://localhost:1984/api/stream.mjpeg?dst=camera1
```
