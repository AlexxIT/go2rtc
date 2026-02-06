# Exec

Exec source can run any external application and expect data from it. Two transports are supported - **pipe** ([`new in v1.5.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)) and **RTSP**.

If you want to use **RTSP** transport, the command must contain the `{output}` argument in any place. On launch, it will be replaced by the local address of the RTSP server.

**pipe** reads data from app stdout in different formats: **MJPEG**, **H.264/H.265 bitstream**, **MPEG-TS**. Also pipe can write data to app stdin in two formats: **PCMA** and **PCM/48000**.

The source can be used with:

- [FFmpeg](https://ffmpeg.org/) - go2rtc ffmpeg source just a shortcut to exec source
- [FFplay](https://ffmpeg.org/ffplay.html) - play audio on your server
- [GStreamer](https://gstreamer.freedesktop.org/)
- [Raspberry Pi Cameras](https://www.raspberrypi.com/documentation/computers/camera_software.html)
- any of your own software

## Configuration

Pipe commands support parameters (format: `exec:{command}#{param1}#{param2}`):

- `killsignal` - signal which will be sent to stop the process (numeric form)
- `killtimeout` - time in seconds for forced termination with sigkill
- `backchannel` - enable backchannel for two-way audio
- `starttimeout` - time in seconds for waiting first byte from RTSP

```yaml
streams:
  stream: exec:ffmpeg -re -i /media/BigBuckBunny.mp4 -c copy -rtsp_transport tcp -f rtsp {output}
  picam_h264: exec:libcamera-vid -t 0 --inline -o -
  picam_mjpeg: exec:libcamera-vid -t 0 --codec mjpeg -o -
  pi5cam_h264: exec:libcamera-vid -t 0 --libav-format h264 -o -
  canon: exec:gphoto2 --capture-movie --stdout#killsignal=2#killtimeout=5
  play_pcma: exec:ffplay -fflags nobuffer -f alaw -ar 8000 -i -#backchannel=1
  play_pcm48k: exec:ffplay -fflags nobuffer -f s16be -ar 48000 -i -#backchannel=1
```

## Backchannel

- You can check audio card names in the **Go2rtc > WebUI > Add**
- You can specify multiple backchannel lines with different codecs

```yaml
sources:
  two_way_audio_win:
    - exec:ffmpeg -hide_banner -f dshow -i "audio=Microphone (High Definition Audio Device)" -c pcm_s16le -ar 16000 -ac 1 -f wav -
    - exec:ffplay -nodisp -probesize 32 -f s16le -ar 16000 -#backchannel=1#audio=s16le/16000
    - exec:ffplay -nodisp -probesize 32 -f alaw -ar 8000 -#backchannel=1#audio=alaw/8000
```
