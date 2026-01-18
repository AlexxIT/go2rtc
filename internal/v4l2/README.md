# V4L2

What you should to know about [V4L2](https://en.wikipedia.org/wiki/Video4Linux):

- V4L2 (Video for Linux API version 2) works only in Linux
- supports USB cameras and other similar devices
- one device can only be connected to one software simultaneously
- cameras support a fixed list of formats, resolutions and frame rates
- basic cameras supports only RAW (non-compressed) pixel formats
- regular cameras supports MJPEG format (series of JPEG frames)
- advances cameras support H264 format (MSE/MP4, WebRTC compatible)
- using MJPEG and H264 formats (if the camera supports them) won't cost you the CPU usage
- transcoding RAW format to MJPEG or H264 - will cost you a significant CPU usage
- H265 (HEVC) format is also supported (if the camera supports it)

Tests show that the basic Keenetic router with MIPS processor can broadcast three MJPEG cameras in the following resolutions: 1600х1200 + 640х480 + 640х480. The USB bus bandwidth is no more enough for larger resolutions. CPU consumption is no more than 5%.

Supported formats for your camera can be found here: **Go2rtc > WebUI > Add > V4L2**.

## RAW format

Example:

```yaml
streams:
  camera1: v4l2:device?video=/dev/video0&input_format=yuyv422&video_size=1280x720&framerate=10
```

Go2rtc supports built-in transcoding of RAW to MJPEG format. This does not need to be additionally configured.

```
ffplay http://localhost:1984/api/stream.mjpeg?src=camera1
```

**Important.** You don't have to transcode the RAW format to transmit it over the network. You can stream it in `y4m` format, which is perfectly supported by ffmpeg. It won't cost you a CPU usage. But will require high network bandwidth.

```
ffplay http://localhost:1984/api/stream.y4m?src=camera1
```
