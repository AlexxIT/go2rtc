# FFmpeg

You can get any stream, file or device via FFmpeg and push it to go2rtc. The app will automatically start FFmpeg with the proper arguments when someone starts watching the stream.

- FFmpeg preinstalled for **Docker** and **Home Assistant add-on** users
- **Home Assistant add-on** users can target files from [/media](https://www.home-assistant.io/more-info/local-media/setup-media/) folder

## Configuration

Format: `ffmpeg:{input}#{param1}#{param2}#{param3}`. Examples:

```yaml
streams:
  # [FILE] all tracks will be copied without transcoding codecs
  file1: ffmpeg:/media/BigBuckBunny.mp4

  # [FILE] video will be transcoded to H264, audio will be skipped
  file2: ffmpeg:/media/BigBuckBunny.mp4#video=h264

  # [FILE] video will be copied, audio will be transcoded to PCMU
  file3: ffmpeg:/media/BigBuckBunny.mp4#video=copy#audio=pcmu

  # [HLS] video will be copied, audio will be skipped
  hls: ffmpeg:https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear5/prog_index.m3u8#video=copy

  # [MJPEG] video will be transcoded to H264
  mjpeg: ffmpeg:http://185.97.122.128/cgi-bin/faststream.jpg#video=h264

  # [RTSP] video with rotation, should be transcoded, so select H264
  rotate: ffmpeg:rtsp://12345678@192.168.1.123/av_stream/ch0#video=h264#rotate=90
```

All transcoding formats have [built-in templates](ffmpeg.go): `h264`, `h265`, `opus`, `pcmu`, `pcmu/16000`, `pcmu/48000`, `pcma`, `pcma/16000`, `pcma/48000`, `aac`, `aac/16000`.

But you can override them via YAML config. You can also add your own formats to the config and use them with source params.

```yaml
ffmpeg:
  bin: ffmpeg  # path to ffmpeg binary
  global: "-hide_banner"
  timeout: 5  # default timeout in seconds for rtsp inputs
  h264: "-codec:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency -profile:v main -level:v 4.1"
  mycodec: "-any args that supported by ffmpeg..."
  myinput: "-fflags nobuffer -flags low_delay -timeout {timeout} -i {input}"
  myraw: "-ss 00:00:20"
```

- You can use go2rtc stream name as ffmpeg input (ex. `ffmpeg:camera1#video=h264`)
- You can use `video` and `audio` params multiple times (ex. `#video=copy#audio=copy#audio=pcmu`)
- You can use `rotate` param with `90`, `180`, `270` or `-90` values, important with transcoding (ex. `#video=h264#rotate=90`)
- You can use `width` and/or `height` params, important with transcoding (ex. `#video=h264#width=1280`)
- You can use `drawtext` to add a timestamp (ex. `drawtext=x=2:y=2:fontsize=12:fontcolor=white:box=1:boxcolor=black`)
    - This will greatly increase the CPU of the server, even with hardware acceleration
- You can use `timeout` param to set RTSP input timeout in seconds (ex. `#timeout=10`)
- You can use `raw` param for any additional FFmpeg arguments (ex. `#raw=-vf transpose=1`)
- You can use `input` param to override default input template (ex. `#input=rtsp/udp` will change RTSP transport from TCP to UDP+TCP)
    - You can use raw input value (ex. `#input=-timeout {timeout} -i {input}`)
    - You can add your own input templates

Read more about [hardware acceleration](hardware/README.md).

**PS.** It is recommended to check the available hardware in the WebUI add page.
