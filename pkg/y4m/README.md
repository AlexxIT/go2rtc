## Planar YUV formats

Packed YUV - yuyv422 - YUYV 4:2:2
Semi-Planar - nv12 - Y/CbCr 4:2:0
Planar YUV - yuv420p - Planar YUV 4:2:0 - aka. [cosited](https://manned.org/yuv4mpeg.5)

```
[video4linux2,v4l2 @ 0x55fddc42a940] Raw       :     yuyv422 :           YUYV 4:2:2 : 1920x1080
[video4linux2,v4l2 @ 0x55fddc42a940] Raw       :        nv12 :         Y/CbCr 4:2:0 : 1920x1080
[video4linux2,v4l2 @ 0x55fddc42a940] Raw       :     yuv420p :     Planar YUV 4:2:0 : 1920x1080
```

## Useful links

- https://learn.microsoft.com/en-us/windows/win32/medfound/recommended-8-bit-yuv-formats-for-video-rendering
- https://developer.mozilla.org/en-US/docs/Web/Media/Formats/Video_concepts
- https://fourcc.org/yuv.php#YV12
- https://docs.kernel.org/userspace-api/media/v4l/pixfmt-yuv-planar.html
- https://gist.github.com/Jim-Bar/3cbba684a71d1a9d468a6711a6eddbeb
