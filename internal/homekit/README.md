# Apple HomeKit

This module supports both client and server for the [Apple HomeKit](https://www.apple.com/home-app/accessories/) protocol.

## HomeKit Client

**Important:**

- You can use HomeKit Cameras **without Apple devices** (iPhone, iPad, etc.), it's just a yet another protocol
- HomeKit device can be paired with only one ecosystem. So, if you have paired it to an iPhone (Apple Home), you can't pair it with Home Assistant or go2rtc. Or if you have paired it to go2rtc, you can't pair it with an iPhone
- HomeKit device should be on the same network with working [mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) between the device and go2rtc

go2rtc supports importing paired HomeKit devices from [Home Assistant](../hass/README.md). 
So you can use HomeKit camera with Home Assistant and go2rtc simultaneously. 
If you are using Home Assistant, I recommend pairing devices with it; it will give you more options.

You can pair device with go2rtc on the HomeKit page. If you can't see your devices, reload the page. 
Also, try rebooting your HomeKit device (power off). If you still can't see it, you have a problem with mDNS.

If you see a device but it does not have a pairing button, it is paired to some ecosystem (Apple Home, Home Assistant, HomeBridge, etc.). You need to delete the device from that ecosystem, and it will be available for pairing. If you cannot unpair the device, you will have to reset it.

**Important:**

- HomeKit audio uses very non-standard **AAC-ELD** codec with very non-standard params and specification violations
- Audio can't be played in `VLC` and probably any other player
- Audio should be transcoded for use with MSE, WebRTC, etc.

### Client Configuration

Recommended settings for using HomeKit Camera with WebRTC, MSE, MP4, RTSP:

```yaml
streams:
  aqara_g3:
    - hass:Camera-Hub-G3-AB12
    - ffmpeg:aqara_g3#audio=aac#audio=opus
```

RTSP link with "normal" audio for any player: `rtsp://192.168.1.123:8554/aqara_g3?video&audio=aac`

**This source is in active development!** Tested only with [Aqara Camera Hub G3](https://www.aqara.com/eu/product/camera-hub-g3) (both EU and CN versions).

## HomeKit Server

[`new in v1.7.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)

HomeKit module can work in two modes:

- export any H264 camera to Apple HomeKit
- transparent proxy any Apple HomeKit camera (Aqara, Eve, Eufy, etc.) back to Apple HomeKit, so you will have all camera features in Apple Home and also will have RTSP/WebRTC/MP4/etc. from your HomeKit camera

**Important**

- HomeKit cameras support only H264 video and OPUS audio

### Server Configuration

**Minimal config**

```yaml
streams:
  dahua1: rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
homekit:
  dahua1:  # same stream ID from streams list, default PIN - 19550224
```

**Full config**

```yaml
streams:
  dahua1:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
    - ffmpeg:dahua1#video=h264#hardware  # if your camera doesn't support H264, important for HomeKit
    - ffmpeg:dahua1#audio=opus           # only OPUS audio supported by HomeKit

homekit:
  dahua1:                   # same stream ID from streams list
    pin: 12345678           # custom PIN, default: 19550224
    name: Dahua camera      # custom camera name, default: generated from stream ID
    device_id: dahua1       # custom ID, default: generated from stream ID
    device_private: dahua1  # custom key, default: generated from stream ID
```

**Proxy HomeKit camera**

- Video stream from HomeKit camera to Apple device (iPhone, AppleTV) will be transmitted directly
- Video stream from HomeKit camera to RTSP/WebRTC/MP4/etc. will be transmitted via go2rtc

```yaml
streams:
  aqara1:
    - homekit://...
    - ffmpeg:aqara1#audio=aac#audio=opus  # optional audio transcoding

homekit:
  aqara1:  # same stream ID from streams list
```
