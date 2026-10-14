# go2rtc (Tapo patch)

[go2rtc](https://github.com/AlexxIT/go2rtc) 1.9.14 with fixes for battery powered TP-Link Tapo cameras and doorbells.
It is a drop-in replacement for the official go2rtc add-on: same config file, same ports, same web interface.

- H265 video and Opus audio over the Tapo protocol (ex. D235 on battery, which has no RTSP)
- Announcements, TTS and radio on the speaker of the camera without stutter
- Several announcements at the same time are mixed
- A snapshot or a closed camera card doesn't cut a running announcement anymore
- No crash on a broken packet from the camera
