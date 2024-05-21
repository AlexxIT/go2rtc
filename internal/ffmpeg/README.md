## FFplay output

[FFplay](https://stackoverflow.com/questions/27778678/what-are-mv-fd-aq-vq-sq-and-f-in-a-video-stream) `7.11 A-V:  0.003 fd=   1 aq=   21KB vq=  321KB sq=    0B f=0/0`:

- `7.11` - master clock, is the time from start of the stream/video
- `A-V` - av_diff, difference between audio and video timestamps
- `fd` - frames dropped
- `aq` - audio queue (0 - no delay)
- `vq` - video queue (0 - no delay)
- `sq` - subtitle queue
- `f` - timestamp error correction rate (Not 100% sure)

`M-V`, `M-A` means video stream only, audio stream only respectively.

## Devices Windows

```
>ffmpeg -hide_banner -f dshow -list_options true -i video="VMware Virtual USB Video Device"
[dshow @ 0000025695e52900] DirectShow video device options (from video devices)
[dshow @ 0000025695e52900]  Pin "Record" (alternative pin name "0")
[dshow @ 0000025695e52900]   pixel_format=yuyv422  min s=1280x720 fps=1 max s=1280x720 fps=10
[dshow @ 0000025695e52900]   pixel_format=yuyv422  min s=1280x720 fps=1 max s=1280x720 fps=10 (tv, bt470bg/bt709/unknown, topleft)
[dshow @ 0000025695e52900]   pixel_format=nv12  min s=1280x720 fps=1 max s=1280x720 fps=23
[dshow @ 0000025695e52900]   pixel_format=nv12  min s=1280x720 fps=1 max s=1280x720 fps=23 (tv, bt470bg/bt709/unknown, topleft)
```

## Devices Mac

```
% ./ffmpeg -hide_banner -f avfoundation -list_devices true -i ""
[AVFoundation indev @ 0x7f8b1f504d80] AVFoundation video devices:
[AVFoundation indev @ 0x7f8b1f504d80] [0] FaceTime HD Camera
[AVFoundation indev @ 0x7f8b1f504d80] [1] Capture screen 0
[AVFoundation indev @ 0x7f8b1f504d80] AVFoundation audio devices:
[AVFoundation indev @ 0x7f8b1f504d80] [0] Soundflower (2ch)
[AVFoundation indev @ 0x7f8b1f504d80] [1] Built-in Microphone
[AVFoundation indev @ 0x7f8b1f504d80] [2] Soundflower (64ch)
```

## Devices Linux 

```
# ffmpeg -hide_banner -f v4l2 -list_formats all -i /dev/video0
[video4linux2,v4l2 @ 0x7f7de7c58bc0] Raw       :     yuyv422 :           YUYV 4:2:2 : 640x480 160x120 176x144 320x176 320x240 352x288 432x240 544x288 640x360 752x416 800x448 800x600 864x480 960x544 960x720 1024x576 1184x656 1280x720 1280x960
[video4linux2,v4l2 @ 0x7f7de7c58bc0] Compressed:       mjpeg :          Motion-JPEG : 640x480 160x120 176x144 320x176 320x240 352x288 432x240 544x288 640x360 752x416 800x448 800x600 864x480 960x544 960x720 1024x576 1184x656 1280x720 1280x960
```

## TTS

```yaml
streams:
  tts: ffmpeg:#input=-readrate 1 -readrate_initial_burst 0.001 -f lavfi -i "flite=text='1 2 3 4 5 6 7 8 9 0'"#audio=pcma
```

## Useful links

- https://superuser.com/questions/564402/explanation-of-x264-tune
- https://stackoverflow.com/questions/33624016/why-sliced-thread-affect-so-much-on-realtime-encoding-using-ffmpeg-x264
- https://codec.fandom.com/ru/wiki/X264_-_описание_ключей_кодирования
- https://html5test.com/
- https://trac.ffmpeg.org/wiki/Capture/Webcam
- https://trac.ffmpeg.org/wiki/DirectShow
- https://stackoverflow.com/questions/53207692/libav-mjpeg-encoding-and-huffman-table
- https://github.com/tuupola/esp_video/blob/master/README.md
- https://github.com/leandromoreira/ffmpeg-libav-tutorial
- https://www.reddit.com/user/VeritablePornocopium/comments/okw130/ffmpeg_with_libfdk_aac_for_windows_x64/
- https://slhck.info/video/2017/02/24/vbr-settings.html
- [HomeKit audio samples problem](https://superuser.com/questions/1290996/non-monotonous-dts-with-igndts-flag)
