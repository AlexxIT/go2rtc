# Notes

go2rtc tries to name formats, protocols and codecs the same way they are named in FFmpeg.
Some formats and protocols go2rtc supports exclusively. They have no equivalent in FFmpeg.

## Producers (input)

- The initiator of the connection can be go2rtc - **Source protocols**
- The initiator of the connection can be an external program - **Ingress protocols**
- Codecs can be incoming - **Recevers codecs**
- Codecs can be outgoing (two way audio) - **Senders codecs**

| Group      | Format     | Protocols     | Ingress | Recevers codecs              | Senders codecs     | Example       |
|------------|------------|---------------|---------|------------------------------|--------------------|---------------|
| Devices    | alsa       | pipe          |         |                              | pcm                | `alsa:`       |
| Devices    | v4l2       | pipe          |         |                              |                    |               |
| Files      | adts       | http,tcp,pipe | http    | aac                          |                    | `http:`       |
| Files      | flv        | http,tcp,pipe | http    | h264,aac                     |                    | `http:`       |
| Files      | h264       | http,tcp,pipe | http    | h264                         |                    | `http:`       |
| Files      | hevc       | http,tcp,pipe | http    | hevc                         |                    | `http:`       |
| Files      | hls        | http          |         | h264,h265,aac,opus           |                    | `http:`       |
| Files      | mjpeg      | http,tcp,pipe | http    | mjpeg                        |                    | `http:`       |
| Files      | mpegts     | http,tcp,pipe | http    | h264,hevc,aac,opus           |                    | `http:`       |
| Files      | mp4        |               |         |                              |                    |               |
| Files      | wav        | http,tcp,pipe | http    | pcm_alaw,pcm_mulaw           |                    | `http:`       |
| Net (pub)  | mpjpeg     | http,tcp,pipe | http    | mjpeg                        |                    | `http:`       |
| Net (pub)  | onvif      | rtsp          |         |                              |                    |               |
| Net (pub)  | rtmp       | rtmp          | rtmp    | h264,aac                     |                    | `rtmp:`       |
| Net (pub)  | rtsp       | rtsp,ws       | rtsp    | h264,hevc,aac,pcm*,opus      | pcm*,opus          | `rtsp:`       |
| Net (pub)  | webrtc*    | webrtc        | webrtc  | h264,pcm_alaw,pcm_mulaw,opus | pcm_alaw,pcm_mulaw | `webrtc:`     |
| Net (pub)  | y4m        | http,tcp,pipe | http    | rawvideo                     |                    | `http:`       |
| Net (priv) | bubble     | http          |         | h264,hevc,pcm_alaw           |                    | `bubble:`     |
| Net (priv) | doorbird   | http          |         |                              |                    |               |
| Net (priv) | dvrip      | tcp           |         | h264,hevc,pcm_alaw,pcm_mulaw | pcm_alaw           | `dvrip:`      |
| Net (priv) | eseecloud  | http          |         |                              |                    |               |
| Net (priv) | gopro      | udp           |         | TODO                         |                    | `gopro:`      |
| Net (priv) | hass       | webrtc        |         | TODO                         |                    | `hass:`       |
| Net (priv) | homekit    | hap           |         | h264,eld*                    |                    | `homekit:`    |
| Net (priv) | isapi      | http          |         |                              | pcm_alaw,pcm_mulaw | `isapi:`      |
| Net (priv) | kasa       | http          |         | h264,pcm_mulaw               |                    | `kasa:`       |
| Net (priv) | nest       | rtsp,webrtc   |         | TODO                         |                    | `nest:`       |
| Net (priv) | ring       | webrtc        |         |                              |                    |               |
| Net (priv) | roborock   | webrtc        |         | h264,opus                    | opus               | `roborock:`   |
| Net (priv) | tapo       | http          |         | h264,pcma                    | pcm_alaw           | `tapo:`       |
| Net (priv) | tuya       | webrtc        |         |                              |                    |               |
| Net (priv) | vigi       | http          |         |                              |                    |               |
| Net (priv) | webtorrent | webrtc        | TODO    | TODO                         | TODO               | `webtorrent:` |
| Net (priv) | xiaomi*    | cs2,tutk      |         |                              |                    |               |
| Services   | flussonic  | ws            |         |                              |                    |               |
| Services   | ivideon    | ws            |         | h264                         |                    | `ivideon:`    |
| Services   | yandex     | webrtc        |         |                              |                    |               |
| Other      | ascii      | http          |         |                              |                    |               |
| Other      | echo       | *             |         |                              |                    |               |
| Other      | exec       | pipe,rtsp     |         |                              |                    |               |
| Other      | expr       | *             |         |                              |                    |               |
| Other      | ffmpeg     | pipe,rtsp     |         |                              |                    |               |
| Other      | stdin      | pipe          |         |                              | pcm_alaw,pcm_mulaw | `stdin:`      |

- **eld** - rare variant of aac codec
- **pcm** - pcm_alaw pcm_mulaw pcm_s16be pcm_s16le
- **webrtc** - webrtc/kinesis, webrtc/openipc, webrtc/milestone, webrtc/wyze, webrtc/whep

## Consumers (output)

| Format       | Protocol    | Send codecs                  | Recv codecs             | Example                               |
|--------------|-------------|------------------------------|-------------------------|---------------------------------------|
| adts         | http        | aac                          |                         | `GET /api/stream.adts`                |
| ascii        | http        | mjpeg                        |                         | `GET /api/stream.ascii`               |
| flv          | http        | h264,aac                     |                         | `GET /api/stream.flv`                 |
| hls/mpegts   | http        | h264,hevc,aac                |                         | `GET /api/stream.m3u8`                |
| hls/fmp4     | http        | h264,hevc,aac,pcm*,opus      |                         | `GET /api/stream.m3u8?mp4`            |
| homekit      | homekit+udp | h264,opus                    |                         | Apple HomeKit app                     |
| mjpeg        | ws          | mjpeg                        |                         | `{"type":"mjpeg"}` -> `/api/ws`       |
| mpjpeg       | http        | mjpeg                        |                         | `GET /api/stream.mjpeg`               |
| mp4          | http        | h264,hevc,aac,pcm*,opus      |                         | `GET /api/stream.mp4`                 |
| mse/fmp4     | ws          | h264,hevc,aac,pcm*,opus      |                         | `{"type":"mse"}` -> `/api/ws`         |
| mpegts       | http        | h264,hevc,aac                |                         | `GET /api/stream.ts`                  |
| rtmp         | rtmp        | h264,aac                     |                         | `rtmp://localhost:1935/{stream_name}` |
| rtsp         | rtsp+tcp    | h264,hevc,aac,pcm*,opus      |                         | `rtsp://localhost:8554/{stream_name}` |
| webrtc       | TODO        | h264,pcm_alaw,pcm_mulaw,opus | pcm_alaw,pcm_mulaw,opus | `{"type":"webrtc"}` -> `/api/ws`      |
| yuv4mpegpipe | http        | rawvideo                     |                         | `GET /api/stream.y4m`                 |

- **pcm** - pcm_alaw pcm_mulaw pcm_s16be pcm_s16le

## Snapshots

| Format | Protocol | Send codecs | Example               |
|--------|----------|-------------|-----------------------|
| jpeg   | http     | mjpeg       | `GET /api/frame.jpeg` |
| mp4    | http     | h264,hevc   | `GET /api/frame.mp4`  |

## Developers

File naming:

- `pkg/{format}/producer.go` - producer for this format (also if support backchannel)
- `pkg/{format}/consumer.go` - consumer for this format
- `pkg/{format}/backchanel.go` - producer with only backchannel func

## Useful links

- https://www.wowza.com/blog/streaming-protocols
- https://vimeo.com/blog/post/rtmp-stream/
- https://sanjeev-pandey.medium.com/understanding-the-mpeg-4-moov-atom-pseudo-streaming-in-mp4-93935e1b9e9a
- [Android Supported media formats](https://developer.android.com/guide/topics/media/media-formats)
- [THEOplayer](https://www.theoplayer.com/test-your-stream-hls-dash-hesp)
- [How Generate DTS/PTS](https://www.ramugedia.com/how-generate-dts-pts-from-elementary-stream)
