# MP4

This module provides several features:

1. MSE stream (fMP4 over WebSocket)
2. Camera snapshots in MP4 format (single frame), can be sent to [Telegram](#snapshot-to-telegram)
3. HTTP progressive streaming (MP4 file stream) - bad format for streaming because of high start delay. This format doesn't work in all Safari browsers, but go2rtc will automatically redirect it to HLS/fMP4 in this case.

## API examples

- MP4 snapshot: `http://192.168.1.123:1984/api/frame.mp4?src=camera1` (H264, H265)
- MP4 stream: `http://192.168.1.123:1984/api/stream.mp4?src=camera1` (H264, H265, AAC)
- MP4 file: `http://192.168.1.123:1984/api/stream.mp4?src=camera1` (H264, H265*, AAC, OPUS, MP3, PCMA, PCMU, PCM)
    - You can use `mp4`, `mp4=flac` and `mp4=all` param for codec filters
    - You can use `duration` param in seconds (ex. `duration=15`)
    - You can use `filename` param (ex. `filename=record.mp4`)
    - You can use `rotate` param with `90`, `180` or `270` values
    - You can use `scale` param with positive integer values (ex. `scale=4:3`)

Read more about [codecs filters](../../README.md#codecs-filters).

**PS.** Rotate and scale params don't use transcoding and change video using metadata.

## Snapshot to Telegram

This examples for Home Assistant [Telegram Bot](https://www.home-assistant.io/integrations/telegram_bot/) integration.

- change `url` to your go2rtc web API (`http://localhost:1984/` for most users)
- change `target` to your Telegram chat ID (support list)
- change `src=camera1` to your stream name from go2rtc config

**Important.** Snapshot will be near instant for most cameras and many sources, except `ffmpeg` source. Because it takes a long time for ffmpeg to start streaming with video, even when you use `#video=copy`. Also the delay can be with cameras that do not start the stream with a keyframe.

### Snapshot from H264 or H265 camera

```yaml
service: telegram_bot.send_video
data:
  url: http://localhost:1984/api/frame.mp4?src=camera1
  target: 123456789
```

### Record from H264 or H265 camera

Record from service call to the future. Doesn't support loopback.

- `mp4=flac` - adds support PCM audio family
- `filename=record.mp4` - set name for downloaded file

```yaml
service: telegram_bot.send_video
data:
  url: http://localhost:1984/api/stream.mp4?src=camera1&mp4=flac&duration=5&filename=record.mp4  # duration in seconds
  target: 123456789
```

### Snapshot from JPEG or MJPEG camera

This example works via the [mjpeg](../mjpeg/README.md) module.

```yaml
service: telegram_bot.send_photo
data:
  url: http://localhost:1984/api/frame.jpeg?src=camera1
  target: 123456789
```
