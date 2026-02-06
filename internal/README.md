# Modules

go2rtc tries to name formats, protocols and codecs the same way they are named in FFmpeg.
Some formats and protocols go2rtc supports exclusively. They have no equivalent in FFmpeg.

- The `echo`, `expr`, `hass` and `onvif` modules receive a link to a stream. They don't know the protocol in advance.
- The `exec` and `ffmpeg` modules support many formats. They are identical to the `http` module.
- The `api`, `app`, `debug`, `ngrok`, `pinggy`, `srtp`, `streams` are supporting modules.

**Modules** implement communication APIs: authorization, encryption, command set, structure of media packets.

**Formats** describe the structure of the data being transmitted.

**Protocols** implement transport for data transmission.

| module       | formats         | protocols        | input | output | ingest | two-way |
|--------------|-----------------|------------------|-------|--------|--------|---------|
| `alsa`       | `pcm`           | `ioctl`          | yes   |        |        |         |
| `bubble`     | -               | `http`           | yes   |        |        |         |
| `doorbird`   | `mulaw`         | `http`           | yes   |        |        | yes     |
| `dvrip`      | -               | `tcp`            | yes   |        |        | yes     |
| `echo`       | *               | *                | yes   |        |        |         |
| `eseecloud`  | `rtp`           | `http`           | yes   |        |        |         |
| `exec`       | *               | `pipe`, `rtsp`   | yes   |        |        | yes     |
| `expr`       | *               | *                | yes   |        |        |         |
| `ffmpeg`     | *               | `pipe`, `rtsp`   | yes   |        |        |         |
| `flussonic`  | `mp4`           | `ws`             | yes   |        |        |         |
| `gopro`      | `mpegts`        | `udp`            | yes   |        |        |         |
| `hass`       | *               | *                | yes   |        |        |         |
| `hls`        | `mpegts`, `mp4` | `http`           |       | yes    |        |         |
| `homekit`    | `rtp`           | `hap`            | yes   | yes    |        | no      |
| `http`       | `adts`          | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `flv`           | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `h264`          | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `hevc`          | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `hls`           | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `mjpeg`         | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `mpjpeg`        | `http`           | yes   |        |        |         |
| `http`       | `mpegts`        | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `wav`           | `http`, `tcp`    | yes   |        |        |         |
| `http`       | `yuv4mpegpipe`  | `http`, `tcp`    | yes   |        |        |         |
| `isapi`      | `alaw`, `mulaw` | `http`           |       |        |        | yes     |
| `ivideon`    | `mp4`           | `ws`             | yes   |        |        |         |
| `mjpeg`      | `ascii`         | `http`           |       | yes    |        |         |
| `mjpeg`      | `jpeg`          | `http`           |       | yes    |        |         |
| `mjpeg`      | `mpjpeg`        | `http`           |       | yes    | yes    |         |
| `mjpeg`      | `yuv4mpegpipe`  | `http`           |       | yes    |        |         |
| `mp4`        | `mp4`           | `http`, `ws`     |       | yes    |        |         |
| `mpegts`     | `adts`          | `http`           |       | yes    |        |         |
| `mpegts`     | `mpegts`        | `http`           |       | yes    | yes    |         |
| `multitrans` | `rtp`           | `tcp`            |       |        |        | yes     |
| `nest`       | `srtp`          | `rtsp`, `webrtc` | yes   |        |        | no      |
| `onvif`      | `rtp`           | *                | yes   | yes    |        |         |
| `ring`       | `srtp`          | `webrtc`         | yes   |        |        | yes     |
| `roborock`   | `srtp`          | `webrtc`         | yes   |        |        | yes     |
| `rtmp`       | `rtmp`          | `rtmp`           | yes   | yes    | yes    |         |
| `rtmp`       | `flv`           | `http`           |       | yes    | yes    |         |
| `rtsp`       | `rtsp`          | `rtsp`           | yes   | yes    | yes    | yes     |
| `tapo`       | `mpegts`        | `http`           | yes   |        |        | yes     |
| `tuya`       | `srtp`          | `webrtc`         | yes   |        |        | yes     |
| `v4l2`       | `rawvideo`      | `ioctl`          | yes   |        |        |         |
| `webrtc`     | `srtp`          | `webrtc`         | yes   | yes    | yes    | yes     |
| `webtorrent` | `srtp`          | `webrtc`         | yes   | yes    |        |         |
| `wyoming`    | `pcm`           | `tcp`            |       | yes    |        |         |
| `wyze`       | -               | `tutk`           | yes   |        |        | yes     |
| `xiaomi`     | -               | `cs2`, `tutk`    | yes   |        |        | yes     |
| `yandex`     | `srtp`          | `webrtc`         | yes   |        |        |         |
