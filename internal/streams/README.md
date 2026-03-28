# Streams

This core module is responsible for managing the stream list.

## Stream to camera

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

go2rtc supports playing audio files (ex. music or [TTS](https://www.home-assistant.io/integrations/#text-to-speech)) and live streams (ex. radio) on cameras with [two-way audio](../../README.md#two-way-audio) support.

API example:

```text
POST http://localhost:1984/api/streams?dst=camera1&src=ffmpeg:http://example.com/song.mp3#audio=pcma#input=file
```

- you can stream: local files, web files, live streams or any format, supported by FFmpeg
- you should use [ffmpeg source](../ffmpeg/README.md) for transcoding audio to codec, that your camera supports
- you can check camera codecs on the go2rtc WebUI info page when the stream is active
- some cameras support only low quality `PCMA/8000` codec (ex. [Tapo](../tapo/README.md))
- it is recommended to choose higher quality formats if your camera supports them (ex. `PCMA/48000` for some Dahua cameras)
- if you play files over `http` link, you need to add `#input=file` params for transcoding, so the file will be transcoded and played in real time
- if you play live streams, you should skip `#input` param, because it is already in real time
- you can stop active playback by calling the API with the empty `src` parameter
- you will see one active producer and one active consumer in go2rtc WebUI info page during streaming

## Publish stream

[`new in v1.8.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)

You can publish any stream to streaming services (YouTube, Telegram, etc.) via RTMP/RTMPS. Important:

- Supported codecs: H264 for video and AAC for audio
- AAC audio is required for YouTube; videos without audio will not work
- You don't need to enable [RTMP module](../rtmp/README.md) listening for this task

You can use the API:

```text
POST http://localhost:1984/api/streams?src=camera1&dst=rtmps://...
```

Or config file:

```yaml
publish:
  # publish stream "video_audio_transcode" to Telegram
  video_audio_transcode:
    - rtmps://xxx-x.rtmp.t.me/s/xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx
  # publish stream "audio_transcode" to Telegram and YouTube
  audio_transcode:
    - rtmps://xxx-x.rtmp.t.me/s/xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx
    - rtmp://xxx.rtmp.youtube.com/live2/xxxx-xxxx-xxxx-xxxx-xxxx

streams:
  video_audio_transcode:
    - ffmpeg:rtsp://user:pass@192.168.1.123/stream1#video=h264#hardware#audio=aac
  audio_transcode:
    - ffmpeg:rtsp://user:pass@192.168.1.123/stream1#video=copy#audio=aac
```

- **Telegram Desktop App** > Any public or private channel or group (where you admin) > Live stream > Start with... > Start streaming.
- **YouTube** > Create > Go live > Stream latency: Ultra low-latency > Copy: Stream URL + Stream key.

### Publish via SOCKS5 proxy

If the streaming service is blocked in your country (e.g. Telegram), you can route the RTMP connection through a SOCKS5 proxy by appending the `socks5` query parameter to the destination URL:

```yaml
publish:
  # publish to Telegram via SOCKS5 proxy (no auth)
  my_stream: rtmps://dc4-1.rtmp.t.me/s/YOUR_KEY#proxy=socks5://proxy.example.com:1080

  # publish to Telegram via SOCKS5 proxy with authentication
  my_stream2: rtmps://dc4-1.rtmp.t.me/s/YOUR_KEY#proxy=socks5://user:pass@proxy.example.com:1080

  # combine with RTMP query parameters
  my_stream3: rtmp://xxx.rtmp.youtube.com/live2/KEY?token=abc#proxy=socks5://proxy.example.com:1080
```

The `socks5` parameter is stripped before the URL is sent to the RTMP server, so it will not affect authentication or stream keys.

## Preload stream

[`new in v1.9.11`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.11)

You can preload any stream on go2rtc start. This is useful for cameras that take a long time to start up.

```yaml
preload:
  camera1:                                     # default: video&audio = ANY
  camera2: "video"                             # preload only video track
  camera3: "video=h264&audio=opus"             # preload H264 video and OPUS audio

streams:
  camera1: 
    - rtsp://192.168.1.100/stream
  camera2: 
    - rtsp://192.168.1.101/stream  
  camera3: 
    - rtsp://192.168.1.102/h265stream
    - ffmpeg:camera3#video=h264#audio=opus#hardware
```

## Examples

```yaml
streams:
  # known RTSP sources
  rtsp-dahua1:   rtsp://admin:password@192.168.10.90/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
  rtsp-dahua2:   rtsp://admin:password@192.168.10.90/cam/realmonitor?channel=1&subtype=1
  rtsp-tplink1:  rtsp://admin:password@192.168.10.91/stream1
  rtsp-tplink2:  rtsp://admin:password@192.168.10.91/stream2
  rtsp-reolink1: rtsp://admin:password@192.168.10.92/h264Preview_01_main
  rtsp-reolink2: rtsp://admin:password@192.168.10.92/h264Preview_01_sub
  rtsp-sonoff1:  rtsp://admin:password@192.168.10.93/av_stream/ch0
  rtsp-sonoff2:  rtsp://admin:password@192.168.10.93/av_stream/ch1

  # known RTMP sources
  rtmp-reolink1: rtmp://192.168.10.92/bcs/channel0_main.bcs?channel=0&stream=0&user=admin&password=password
  rtmp-reolink2: rtmp://192.168.10.92/bcs/channel0_sub.bcs?channel=0&stream=1&user=admin&password=password
  rtmp-reolink3: rtmp://192.168.10.92/bcs/channel0_ext.bcs?channel=0&stream=1&user=admin&password=password

  # known HTTP sources
  http-reolink1: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_main.bcs&user=admin&password=password
  http-reolink2: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_sub.bcs&user=admin&password=password
  http-reolink3: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_ext.bcs&user=admin&password=password

  # known ONVIF sources
  onvif-dahua1:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00000
  onvif-dahua2:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00001
  onvif-dahua3:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00000&snapshot
  onvif-tplink1:  onvif://admin:password@192.168.10.91:2020?subtype=profile_1
  onvif-tplink2:  onvif://admin:password@192.168.10.91:2020?subtype=profile_2
  onvif-reolink1: onvif://admin:password@192.168.10.92:8000?subtype=000
  onvif-reolink2: onvif://admin:password@192.168.10.92:8000?subtype=001
  onvif-reolink3: onvif://admin:password@192.168.10.92:8000?subtype=000&snapshot
  onvif-openipc1: onvif://admin:password@192.168.10.95:80?subtype=PROFILE_000
  onvif-openipc2: onvif://admin:password@192.168.10.95:80?subtype=PROFILE_001

  # some EXEC examples
  exec-h264-pipe:   exec:ffmpeg -re -i bbb.mp4 -c copy -f h264 -
  exec-flv-pipe:    exec:ffmpeg -re -i bbb.mp4 -c copy -f flv -
  exec-mpegts-pipe: exec:ffmpeg -re -i bbb.mp4 -c copy -f mpegts -
  exec-adts-pipe:   exec:ffmpeg -re -i bbb.mp4 -c copy -f adts -
  exec-mjpeg-pipe:  exec:ffmpeg -re -i bbb.mp4 -c mjpeg -f mjpeg -
  exec-hevc-pipe:   exec:ffmpeg -re -i bbb.mp4 -c libx265 -preset superfast -tune zerolatency -f hevc -
  exec-wav-pipe:    exec:ffmpeg -re -i bbb.mp4 -c pcm_alaw -ar 8000 -ac 1 -f wav -
  exec-y4m-pipe:    exec:ffmpeg -re -i bbb.mp4 -c rawvideo -f yuv4mpegpipe -
  exec-pcma-pipe:   exec:ffmpeg -re -i numb.mp3 -c:a pcm_alaw -ar:a 8000 -ac:a 1 -f wav -
  exec-pcmu-pipe:   exec:ffmpeg -re -i numb.mp3 -c:a pcm_mulaw -ar:a 8000 -ac:a 1 -f wav -
  exec-s16le-pipe:  exec:ffmpeg -re -i numb.mp3 -c:a pcm_s16le -ar:a 16000 -ac:a 1 -f wav -

  # some FFmpeg examples
  ffmpeg-video-h264: ffmpeg:virtual?video#video=h264
  ffmpeg-video-4K:   ffmpeg:virtual?video&size=4K#video=h264
  ffmpeg-video-10s:  ffmpeg:virtual?video&duration=10#video=h264
  ffmpeg-video-src2: ffmpeg:virtual?video=testsrc2&size=2K#video=h264
```
