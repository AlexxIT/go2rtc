# Hass

Support import camera links from [Home Assistant](https://www.home-assistant.io/) config files:

- [Generic Camera](https://www.home-assistant.io/integrations/generic/), setup via GUI
- [HomeKit Camera](https://www.home-assistant.io/integrations/homekit_controller/)
- [ONVIF](https://www.home-assistant.io/integrations/onvif/)
- [Roborock](https://github.com/humbertogontijo/homeassistant-roborock) vacuums with camera

## Configuration

```yaml
hass:
  config: "/config"  # skip this setting if you are a Home Assistant add-on user

streams:
  generic_camera: hass:Camera1  # Settings > Integrations > Integration Name
  aqara_g3: hass:Camera-Hub-G3-AB12
```

### WebRTC Cameras

[`new in v1.6.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.0)

Any cameras in WebRTC format are supported. But at the moment Home Assistant only supports some [Nest](https://www.home-assistant.io/integrations/nest/) cameras in this format.

**Important.** The Nest API only allows you to get a link to a stream for 5 minutes.
Do not use this with Frigate! If the stream expires, Frigate will consume all available RAM on your machine within seconds.
It's recommended to use [Nest source](../nest/README.md) - it supports extending the stream.

```yaml
streams:
  # link to Home Assistant Supervised
  hass-webrtc1: hass://supervisor?entity_id=camera.nest_doorbell
  # link to external Home Assistant with Long-Lived Access Tokens
  hass-webrtc2: hass://192.168.1.123:8123?entity_id=camera.nest_doorbell&token=eyXYZ...
```

### RTSP Cameras

By default, the Home Assistant API does not allow you to get a dynamic RTSP link to a camera stream. [This method](https://github.com/felipecrs/hass-expose-camera-stream-source#importing-cameras-from-home-assistant-to-go2rtc-or-frigate) can work around it.
