# Tuya

[`new in v1.9.13`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13) by [@seydx](https://github.com/seydx)

[Tuya](https://www.tuya.com/) is a proprietary camera protocol with **two-way audio** support. go2rtc supports `Tuya Smart API`, `Tuya Cloud API` and `Tuya LAN API` (local, no account needed).

**Tuya Smart API (recommended)**:
- **Smart Life accounts are NOT supported**, you need to create a Tuya Smart account. If the cameras are already added to the Smart Life app, you need to remove them and add them again to the [Tuya Smart](https://play.google.com/store/apps/details?id=com.tuya.smart) app.
- Cameras can be discovered through the go2rtc web interface via Tuya Smart account (Add > Tuya > Select region and fill in email and password > Login).

**Tuya Cloud API**:
- Requires setting up a cloud project in the Tuya Developer Platform.
- Obtain `device_id`, `client_id`, `client_secret`, and `uid` from [Tuya IoT Platform](https://iot.tuya.com/). [Here's a guide](https://xzetsubou.github.io/hass-localtuya/cloud_api/).
- Please ensure that you have subscribed to the `IoT Video Live Stream` service (Free Trial) in the Tuya Developer Platform, otherwise the stream will not work (Tuya Developer Platform > Service API > Authorize > IoT Video Live Stream).

## Configuration

### Tuya LAN API (no account required)

Cameras on the same network can be used **without any Tuya account, cloud project or internet
access**. They speak the same signaling protocol over TCP port 6668, authenticated with their
16-byte device local key, and the media still flows over WebRTC. Only the device local key (and,
for some cameras, the WebRTC session parameters) has to be fetched from the cloud once.

```yaml
streams:
  camera:
    # video + camera microphone
    - rtsp://user:pass@192.168.1.10:554/live/ch0
    # talkback: client microphone -> camera speaker over the Tuya LAN protocol
    - tuya-lan://192.168.1.10?device_id=XXX&local_key=XXXXXXXXXXXXXXXX
```

The URL host is the camera address. Parameters (all optional except `device_id` and `local_key`):
- `local_key` - 16 byte device local key (Tuya IoT Platform > device > `Get Device Information`,
  or `tuya-cli wizard`).
- `port` - signaling port, default `6668`.
- `hgw_version` - protocol version, only `3.4` is implemented (default).
- `stun` - ICE server the camera uses for its own candidates, e.g.
  `stun:192.168.1.5:3478`. Needed when the camera cannot reach the go2rtc host directly
  (different subnet/VLAN) - without it such cameras never complete ICE.
- `sender_id` - client identifier, generated when missing.
- `skill`, `webrtc_auth`, `moto_id`, `protocol_version` - WebRTC session parameters that the
  Tuya apps fetch from the cloud (`GET /v1.0/devices/{device_id}/webrtc-configs`). Optional for
  standard cameras, required by cameras that check the app handshake.

Instead of the query string, all parameters can live in a JSON file, which keeps the local key
out of the go2rtc config (file permissions apply):

```yaml
streams:
  camera:
    - tuya-lan:/etc/go2rtc/tuya-lan.json
```

```json
{
  "camera_ip": "192.168.1.10",
  "device_id": "XXX",
  "local_key": "XXXXXXXXXXXXXXXX",
  "sender_id": "az1671493732456NEtjB",
  "stun_url": "stun:192.168.1.5:3478"
}
```

Notes:
- The LAN video track uses Tuya's private KCP transport, which go2rtc cannot decode. Keep a
  standard RTSP/ONVIF source first for video and use the LAN source for two-way audio.
- Most cameras accept a single audio session at a time: while go2rtc holds the LAN session, the
  camera may stop sending audio over RTSP, and a second client may get no audio at all.
- A camera that accepts signaling but sends no media is retried with a per-device exponential
  backoff (up to two minutes), allowing its small session table to drain without delaying other
  Tuya LAN cameras. These timeouts do not change the existing Smart or Cloud API transports.

### Tuya Smart API / Cloud API

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
