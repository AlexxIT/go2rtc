# Real Time Streaming Protocol

This module provides the following features for the RTSP protocol:

 - Streaming input - [RTSP client](#rtsp-client)
 - Streaming output - [RTSP server](#rtsp-server)
 - [Streaming ingest](#streaming-ingest)
 - [Two-way audio](#two-way-audio)

## RTSP Client

### Configuration

```yaml
streams:
  sonoff_camera: rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0
  dahua_camera:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=1#backchannel=0
  amcrest_doorbell:
    - rtsp://username:password@192.168.1.123:554/cam/realmonitor?channel=1&subtype=0#backchannel=0
  unifi_camera: rtspx://192.168.1.123:7441/fD6ouM72bWoFijxK
  glichy_camera: ffmpeg:rtsp://username:password@192.168.1.123/live/ch00_1 
```

### Recommendations

- **Amcrest Doorbell** users may want to disable two-way audio, because with an active stream, you won't have a working call button. You need to add `#backchannel=0` to the end of your RTSP link in YAML config file
- **Dahua Doorbell** users may want to change [audio codec](https://github.com/AlexxIT/go2rtc/issues/49#issuecomment-2127107379) for proper two-way audio. Make sure not to request backchannel multiple times by adding `#backchannel=0` to other stream sources of the same doorbell. The `unicast=true&proto=Onvif` is preferred for two-way audio as this makes the doorbell accept multiple codecs for the incoming audio
- **Reolink** users may want NOT to use RTSP protocol at all, some camera models have a very awful, unusable stream implementation
- **Ubiquiti UniFi** users may want to disable HTTPS verification. Use `rtspx://` prefix instead of `rtsps://`. And don't use `?enableSrtp` [suffix](https://github.com/AlexxIT/go2rtc/issues/81)
- **TP-Link Tapo** users may skip login and password, because go2rtc supports login [without them](https://drmnsamoliu.github.io/video.html)
- If your camera has two RTSP links, you can add both as sources. This is useful when streams have different codecs, for example AAC audio with main stream and PCMU/PCMA audio with second stream
- If the stream from your camera is glitchy, try using [ffmpeg source](../ffmpeg/README.md). It will not add CPU load if you don't use transcoding
- If the stream from your camera is very glitchy, try to use transcoding with [ffmpeg source](../ffmpeg/README.md)

### Other options

Format: `rtsp...#{param1}#{param2}#{param3}`

- Add custom timeout `#timeout=30` (in seconds)
- Ignore audio - `#media=video` or ignore video - `#media=audio`
- Ignore two-way audio API `#backchannel=0` - important for some glitchy cameras
- Use WebSocket transport `#transport=ws...`

### RTSP over WebSocket

```yaml
streams:
  # WebSocket with authorization, RTSP - without
  axis-rtsp-ws:  rtsp://192.168.1.123:4567/axis-media/media.amp?overview=0&camera=1&resolution=1280x720&videoframeskipmode=empty&Axis-Orig-Sw=true#transport=ws://user:pass@192.168.1.123:4567/rtsp-over-websocket
  # WebSocket without authorization, RTSP - with
  dahua-rtsp-ws: rtsp://user:pass@192.168.1.123/cam/realmonitor?channel=1&subtype=1&proto=Private3#transport=ws://192.168.1.123/rtspoverwebsocket
```

## RTSP Server

You can get any stream as RTSP-stream: `rtsp://192.168.1.123:8554/{stream_name}`

You can enable external password protection for your RTSP streams. Password protection is always disabled for localhost calls (ex. FFmpeg or Home Assistant on the same server).

### Configuration

```yaml
rtsp:
  listen: ":8554"    # RTSP Server TCP port, default - 8554
  username: "admin"  # optional, default - disabled
  password: "pass"   # optional, default - disabled
  default_query: "video&audio"  # optional, default codecs filters 
```

By default go2rtc provide RTSP-stream with only one first video and only one first audio. You can change it with the `default_query` setting:

- `default_query: "mp4"` - MP4 compatible codecs (H264, H265, AAC)
- `default_query: "video=all&audio=all"` - all tracks from all source (not all players can handle this)
- `default_query: "video=h264,h265"` - only one video track (H264 or H265)
- `default_query: "video&audio=all"` - only one first any video and all audio as separate tracks

Read more about [codecs filters](../../README.md#codecs-filters).

## Streaming ingest

```shell
ffmpeg -re -i BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp rtsp://localhost:8554/camera1
```

## Two-way audio

Before purchasing, it is difficult to understand whether the camera supports two-way audio via the RTSP protocol or not. This isn't usually mentioned in a camera's description. You can only find out by reading reviews from real buyers.

A camera is considered to support two-way audio if it supports the ONVIF Profile T protocol. But in reality, this isn't always the case. And the ONVIF protocol has no connection with the camera's RTSP implementation.

In go2rtc you can find out if the camera supports two-way audio via WebUI > stream probe.
