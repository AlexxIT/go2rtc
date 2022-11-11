## HEVC

Browser     | avc1 | hvc1 | hev1
------------|------|------|---
Mac Chrome  | +    | -    | +
Mac Safari  | +    | +    | -
iOS 15?     | +    | +    | -
Mac Firefox | +    | -    | -
iOS 12      | +    | -    | -
Android 13  | +    | -    | -

```
ffmpeg -i input-hev1.mp4 -c:v copy -tag:v hvc1 -c:a copy output-hvc1.mp4
Stream #0:0(eng): Video: hevc (Main) (hev1 / 0x31766568), yuv420p(tv, progressive), 720x404, 164 kb/s, 29.97 fps,
Stream #0:0(eng): Video: hevc (Main) (hvc1 / 0x31637668), yuv420p(tv, progressive), 720x404, 164 kb/s, 29.97 fps,
```

## Useful links

- https://stackoverflow.com/questions/63468587/what-hevc-codec-tag-to-use-with-fmp4-hvc1-or-hev1
- https://stackoverflow.com/questions/32152090/encode-h265-to-hvc1-codec
- https://jellyfin.org/docs/general/clients/codec-support.html
- https://github.com/StaZhu/enable-chromium-hevc-hardware-decoding
- https://developer.mozilla.org/ru/docs/Web/Media/Formats/codecs_parameter
