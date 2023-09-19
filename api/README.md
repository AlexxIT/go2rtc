# API

Fill free to make any API design proposals.

## HTTP API

Interactive [OpenAPI](https://alexxit.github.io/go2rtc/api/).

`www/stream.html` - universal viewer with support params in URL:

- multiple streams on page `src=camera1&src=camera2...`
- stream technology autoselection `mode=webrtc,webrtc/tcp,mse,hls,mp4,mjpeg`
- stream technology comparison `src=camera1&mode=webrtc&mode=mse&mode=mp4`
- player width setting in pixels `width=320px` or percents `width=50%`

`www/webrtc.html` - WebRTC viewer with support two way audio and params in URL:

- `media=video+audio` - simple viewer
- `media=video+audio+microphone` - two way audio from camera
- `media=camera+microphone` - stream from browser
- `media=display+speaker` - stream from desktop

## JavaScript API

- You can write your viewer from the scratch
- You can extend the built-in viewer - `www/video-rtc.js`
- Check example - `www/video-stream.js`
- Check example - https://github.com/AlexxIT/WebRTC

`video-rtc.js` features:

- support technologies:
    - WebRTC over UDP or TCP
    - MSE or HLS or MP4 or MJPEG over WebSocket
- automatic selection best technology according on:
    - codecs inside your stream
    - current browser capabilities
    - current network configuration
- automatic stop stream while browser or page not active
- automatic stop stream while player not inside page viewport
- automatic reconnection

Technology selection based on priorities:

1. Video and Audio better than just Video
2. H265 better than H264
3. WebRTC better than MSE, than HLS, than MJPEG

## WebSocket API

Endpoint: `/api/ws`

Query parameters:

- `src` (required) - Stream name

### WebRTC

Request SDP:

```json
{"type":"webrtc/offer","value":"v=0\r\n..."}
```

Response SDP:

```json
{"type":"webrtc/answer","value":"v=0\r\n..."}
```

Request/response candidate:

- empty value also allowed and optional

```json
{"type":"webrtc/candidate","value":"candidate:3277516026 1 udp 2130706431 192.168.1.123 54321 typ host"}
```

### MSE

Request:

- codecs list optional

```json
{"type":"mse","value":"avc1.640029,avc1.64002A,avc1.640033,hvc1.1.6.L153.B0,mp4a.40.2,mp4a.40.5,flac,opus"}
```

Response:

```json
{"type":"mse","value":"video/mp4; codecs=\"avc1.64001F,mp4a.40.2\""}
```

### HLS

Request:

```json
{"type":"hls","value":"avc1.640029,avc1.64002A,avc1.640033,hvc1.1.6.L153.B0,mp4a.40.2,mp4a.40.5,flac"}
```

Response:

- you MUST rewrite full HTTP path to `http://192.168.1.123:1984/api/hls/playlist.m3u8`

```json
{"type":"hls","value":"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000,CODECS=\"avc1.64001F,mp4a.40.2\"\nhls/playlist.m3u8?id=DvmHdd9w"}
```

### MJPEG

Request/response:

```json
{"type":"mjpeg"}
```
