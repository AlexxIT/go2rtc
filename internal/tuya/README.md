# Tuya

[`new in v1.9.13`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13) by [@seydx](https://github.com/seydx)

[Tuya](https://www.tuya.com/) is a proprietary camera protocol with **two-way audio** support. go2rtc supports `Tuya Smart API` and `Tuya Cloud API`.

**Tuya Smart API (recommended)**:
- **Smart Life accounts are NOT supported**, you need to create a Tuya Smart account. If the cameras are already added to the Smart Life app, you need to remove them and add them again to the [Tuya Smart](https://play.google.com/store/apps/details?id=com.tuya.smart) app.
- Cameras can be discovered through the go2rtc web interface via Tuya Smart account (Add > Tuya > Select region and fill in email and password > Login).

**Tuya Cloud API**:
- Requires setting up a cloud project in the Tuya Developer Platform.
- Obtain `device_id`, `client_id`, `client_secret`, and `uid` from [Tuya IoT Platform](https://iot.tuya.com/). [Here's a guide](https://xzetsubou.github.io/hass-localtuya/cloud_api/).
- Please ensure that you have subscribed to the `IoT Video Live Stream` service (Free Trial) in the Tuya Developer Platform, otherwise the stream will not work (Tuya Developer Platform > Service API > Authorize > IoT Video Live Stream).

## Configuration

Use the `resolution` parameter to select the stream (not all cameras support an `hd` stream through WebRTC even if the camera supports it):
- `hd` - HD stream (default)
- `sd` - SD stream

```yaml
streams:
  # Tuya Smart API: WebRTC main stream (use Add > Tuya to discover the URL)
  tuya_main:
    - tuya://protect-us.ismartlife.me?device_id=XXX&email=XXX&password=XXX

  # Tuya Smart API: WebRTC sub stream (use Add > Tuya to discover the URL)
  tuya_sub:
    - tuya://protect-us.ismartlife.me?device_id=XXX&email=XXX&password=XXX&resolution=sd

  # Tuya Cloud API: WebRTC main stream
  tuya_webrtc:
   - tuya://openapi.tuyaus.com?device_id=XXX&uid=XXX&client_id=XXX&client_secret=XXX
  
  # Tuya Cloud API: WebRTC sub stream
  tuya_webrtc_sd:
   - tuya://openapi.tuyaus.com?device_id=XXX&uid=XXX&client_id=XXX&client_secret=XXX&resolution=sd
```
