# FFmpeg Device

You can get video from any USB camera or Webcam as RTSP or WebRTC stream. This is part of FFmpeg integration.

- check available devices in web interface
- `video_size` and `framerate` must be supported by your camera!
- for Linux supported only video for now
- for macOS you can stream FaceTime camera or whole desktop!
- for macOS important to set right framerate

## Configuration

Format: `ffmpeg:device?{input-params}#{param1}#{param2}#{param3}`

```yaml
streams:
  linux_usbcam:   ffmpeg:device?video=0&video_size=1280x720#video=h264
  windows_webcam: ffmpeg:device?video=0#video=h264
  macos_facetime: ffmpeg:device?video=0&audio=1&video_size=1280x720&framerate=30#video=h264#audio=pcma
  macos_serial:   ffmpeg:device?video_device_id=serial:ABCD1234&video_size=1280x720&framerate=30#video=h264
```

On macOS, `video` and `audio` select a device by name or index and
`video_device_id` and `audio_device_id` select a device by AVFoundation unique ID

**PS.** It is recommended to check the available devices in the WebUI add page.
