# HLS

[`new in v1.1.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.1.0)

[HLS](https://en.wikipedia.org/wiki/HTTP_Live_Streaming) is the worst technology for real-time streaming. 
It can only be useful on devices that do not support more modern technology, like [WebRTC](../webrtc/README.md), [MP4](../mp4/README.md).

The go2rtc implementation differs from the standards and may not work with all players.

API examples:

- HLS/TS stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1` (H264)
- HLS/fMP4 stream: `http://192.168.1.123:1984/api/stream.m3u8?src=camera1&mp4` (H264, H265, AAC)

Read more about [codecs filters](../../README.md#codecs-filters).

## Useful links

- https://walterebert.com/playground/video/hls/
