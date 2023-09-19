## Fragmented MP4

```
ffmpeg -i "rtsp://..." -movflags +frag_keyframe+separate_moof+default_base_moof+empty_moov -frag_duration 1 -c copy -t 5 sample.mp4
```

- movflags frag_keyframe 
  Start a new fragment at each video keyframe.
- frag_duration duration
  Create fragments that are duration microseconds long.
- movflags separate_moof
  Write a separate moof (movie fragment) atom for each track.
- movflags default_base_moof
  Similarly to the omit_tfhd_offset, this flag avoids writing the absolute base_data_offset field in tfhd atoms, but does so by using the new default-base-is-moof flag instead.

https://ffmpeg.org/ffmpeg-formats.html#Options-13

## HEVC

| Browser     | avc1 | hvc1 | hev1 |
|-------------|------|------|------|
 | Mac Chrome  | +    | -    | +    |
 | Mac Safari  | +    | +    | -    |
 | iOS 15?     | +    | +    | -    |
 | Mac Firefox | +    | -    | -    |
 | iOS 12      | +    | -    | -    |
 | Android 13  | +    | -    | -    |

## Useful links

- https://stackoverflow.com/questions/63468587/what-hevc-codec-tag-to-use-with-fmp4-hvc1-or-hev1
- https://stackoverflow.com/questions/32152090/encode-h265-to-hvc1-codec
- https://jellyfin.org/docs/general/clients/codec-support.html
- https://github.com/StaZhu/enable-chromium-hevc-hardware-decoding
- https://developer.mozilla.org/ru/docs/Web/Media/Formats/codecs_parameter
- https://gstreamer-devel.narkive.com/rhkUolp2/rtp-dts-pts-result-in-varying-mp4-frame-durations
