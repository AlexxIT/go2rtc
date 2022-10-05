# H264

Access Unit (AU) can contain one or multiple NAL Unit:

1. [SEI,] SPS, PPS, IFrame, [IFrame...]
2. BFrame, [BFrame...]
3. IFrame, [IFrame...]

## RTP H264

Camera | NALu
-------|-----
EZVIZ C3S          | 7f, 8f, 28:28:28 -> 5t, 28:28:28 -> 1t, 1t, 1t, 1t
Sonoff GK-200MP2-B | 28:28:28 -> 5t, 1t, 1t, 1t
Dahua IPC-K42      | 7f, 8f, 28:28:28 -> 5t, 28:28:28 -> 1t, 28:28:28 -> 1t
FFmpeg copy        | 5t, 1t, 1t, 28:28:28 -> 1t, 28:28:28 -> 1t
FFmpeg h264        | 24 -> 6:5:5:5:5t, 24 -> 1:1:1:1t, 28:28:28 -> 5f, 28:28:28 -> 5f, 28:28:28 -> 5t
FFmpeg resize      | 6f, 28:28:28 -> 5f, 28... -> 5t, 24 -> 1:1f, 24 -> 1:1t

## WebRTC

Video codec	    | Media string | Device
----------------|--------------|-------
H.264/baseline! | avc1.42E0xx  | Chromecast
H.264/baseline! | avc1.42E0xx  | Chrome/Safari WebRTC
H.264/baseline! | avc1.42C0xx  | FFmpeg ultrafast
H.264/baseline! | avc1.4240xx  | Dahua H264B
H.264/baseline  | avc1.4200xx  | Chrome WebRTC
H.264/main!     | avc1.4D40xx  | Chromecast
H.264/main!     | avc1.4D40xx  | FFmpeg superfast main
H.264/main!     | avc1.4D40xx  | Dahua H264
H.264/main      | avc1.4D00xx  | Chrome WebRTC
H.264/high!     | avc1.640Cxx  | Safari WebRTC
H.264/high      | avc1.6400xx  | Chromecast
H.264/high      | avc1.6400xx  | FFmpeg superfast

## Useful Links

- [RTP Payload Format for H.264 Video](https://datatracker.ietf.org/doc/html/rfc6184)
- [The H264 Sequence parameter set](https://www.cardinalpeak.com/blog/the-h-264-sequence-parameter-set)
- [H.264 Video Types (Microsoft)](https://docs.microsoft.com/en-us/windows/win32/directshow/h-264-video-types)
- [Automatic Generation of H.264 Parameter Sets to Recover Video File Fragments](https://arxiv.org/pdf/2104.14522.pdf)
- [Chromium sources](https://chromium.googlesource.com/external/webrtc/+/HEAD/common_video/h264)
- [AVC levels](https://en.wikipedia.org/wiki/Advanced_Video_Coding#Levels)
- [AVC profiles table](https://developer.mozilla.org/ru/docs/Web/Media/Formats/codecs_parameter)
- [Supported Media for Google Cast](https://developers.google.com/cast/docs/media)
- [Two stream formats, Annex-B, AVCC (H.264) and HVCC (H.265)](https://www.programmersought.com/article/3901815022/)
