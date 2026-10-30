# Reolink

[`new in v1.9.16`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.16)

This source type supports only two-way audio for [Reolink](https://reolink.com/) cameras, over Reolink's own
protocol (TCP port 9000), which the Reolink app uses as well. Many Reolink models have a speaker but no ONVIF
backchannel on their RTSP server, so the RTSP source cannot talk to them. So it should be used as a second source in
addition to the video stream.

## Configuration

```yaml
streams:
  reolink_camera:
    - rtsp://admin:password@192.168.1.123/Preview_01_main
    - reolink://admin:password@192.168.1.123
```

- The default port is 9000, add `:port` when the camera uses another one.
- The camera tells which audio it accepts (IMA ADPCM, 16 kHz, mono on the tested models). go2rtc accepts PCM, PCMA
  and PCMU from the browser or from FFmpeg and converts it.
- The talk session opens with the first audio and closes after 2 seconds of silence, so the Reolink app can talk again.
- Audio must arrive in real time. A faster source is slowed down, and packets that do not fit in the buffer are lost.
- A camera without a speaker still reports that it can talk. Only add this source to cameras that have one.

Tested with RLC-833A.
