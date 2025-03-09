## Examples

```yaml
streams:
  # known RTSP sources
  rtsp-dahua1:   rtsp://admin:password@192.168.10.90/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif
  rtsp-dahua2:   rtsp://admin:password@192.168.10.90/cam/realmonitor?channel=1&subtype=1
  rtsp-tplink1:  rtsp://admin:password@192.168.10.91/stream1
  rtsp-tplink2:  rtsp://admin:password@192.168.10.91/stream2
  rtsp-reolink1: rtsp://admin:password@192.168.10.92/h264Preview_01_main
  rtsp-reolink2: rtsp://admin:password@192.168.10.92/h264Preview_01_sub
  rtsp-sonoff1:  rtsp://admin:password@192.168.10.93/av_stream/ch0
  rtsp-sonoff2:  rtsp://admin:password@192.168.10.93/av_stream/ch1

  # known RTMP sources
  rtmp-reolink1: rtmp://192.168.10.92/bcs/channel0_main.bcs?channel=0&stream=0&user=admin&password=password
  rtmp-reolink2: rtmp://192.168.10.92/bcs/channel0_sub.bcs?channel=0&stream=1&user=admin&password=password
  rtmp-reolink3: rtmp://192.168.10.92/bcs/channel0_ext.bcs?channel=0&stream=1&user=admin&password=password

  # known HTTP sources
  http-reolink1: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_main.bcs&user=admin&password=password
  http-reolink2: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_sub.bcs&user=admin&password=password
  http-reolink3: http://192.168.10.92/flv?port=1935&app=bcs&stream=channel0_ext.bcs&user=admin&password=password

  # known ONVIF sources
  onvif-dahua1:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00000
  onvif-dahua2:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00001
  onvif-dahua3:   onvif://admin:password@192.168.10.90?subtype=MediaProfile00000&snapshot
  onvif-tplink1:  onvif://admin:password@192.168.10.91:2020?subtype=profile_1
  onvif-tplink2:  onvif://admin:password@192.168.10.91:2020?subtype=profile_2
  onvif-reolink1: onvif://admin:password@192.168.10.92:8000?subtype=000
  onvif-reolink2: onvif://admin:password@192.168.10.92:8000?subtype=001
  onvif-reolink3: onvif://admin:password@192.168.10.92:8000?subtype=000&snapshot
  onvif-openipc1: onvif://admin:password@192.168.10.95:80?subtype=PROFILE_000
  onvif-openipc2: onvif://admin:password@192.168.10.95:80?subtype=PROFILE_001

  # some EXEC examples
  exec-h264-pipe:   exec:ffmpeg -re -i bbb.mp4 -c copy -f h264 -
  exec-flv-pipe:    exec:ffmpeg -re -i bbb.mp4 -c copy -f flv -
  exec-mpegts-pipe: exec:ffmpeg -re -i bbb.mp4 -c copy -f mpegts -
  exec-adts-pipe:   exec:ffmpeg -re -i bbb.mp4 -c copy -f adts -
  exec-mjpeg-pipe:  exec:ffmpeg -re -i bbb.mp4 -c mjpeg -f mjpeg -
  exec-hevc-pipe:   exec:ffmpeg -re -i bbb.mp4 -c libx265 -preset superfast -tune zerolatency -f hevc -
  exec-wav-pipe:    exec:ffmpeg -re -i bbb.mp4 -c pcm_alaw -ar 8000 -ac 1 -f wav -
  exec-y4m-pipe:    exec:ffmpeg -re -i bbb.mp4 -c rawvideo -f yuv4mpegpipe -
  exec-pcma-pipe:   exec:ffmpeg -re -i numb.mp3 -c:a pcm_alaw -ar:a 8000 -ac:a 1 -f wav -
  exec-pcmu-pipe:   exec:ffmpeg -re -i numb.mp3 -c:a pcm_mulaw -ar:a 8000 -ac:a 1 -f wav -
  exec-s16le-pipe:  exec:ffmpeg -re -i numb.mp3 -c:a pcm_s16le -ar:a 16000 -ac:a 1 -f wav -

  # some FFmpeg examples
  ffmpeg-video-h264: ffmpeg:virtual?video#video=h264
  ffmpeg-video-4K:   ffmpeg:virtual?video&size=4K#video=h264
  ffmpeg-video-10s:  ffmpeg:virtual?video&duration=10#video=h264
  ffmpeg-video-src2: ffmpeg:virtual?video=testsrc2&size=2K#video=h264
```
