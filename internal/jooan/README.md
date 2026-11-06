# JOOAN

The `jooan` source provides local-only talkback for compatible JOOAN cameras
using their native TUTK/DTLS protocol. It has been tested with CAM720 cameras.
It does not use cloud services or provide video; combine it with the camera's
RTSP source.

Configure each camera as an alias so credentials and device identifiers are not
placed in stream URLs or connection diagnostics.

```yaml
jooan:
  patio_cam:
    host: 192.0.2.10
    port: 32761
    uid: CAM720-UID-EXAMPLE
    username: admin
    password: replace-with-a-local-secret
    speaker_channel: 1

streams:
  patio_view: rtsp://192.0.2.10:8554/live
  patio_combined:
    - rtsp://192.0.2.10:8554/live
    - jooan:patio_cam
```

The source accepts PCMU audio at 16 kHz, mono, and supports one active talkback
sender. It packetizes the audio into 40 ms frames for the camera speaker.

| Option | Default | Description |
|--------|---------|-------------|
| `host` | required | Camera LAN hostname or IP address |
| `port` | `32761` | Camera TUTK UDP port |
| `uid` | required | Camera TUTK device identifier |
| `username` | required | Local AV account username |
| `password` | required | Local AV account password |
| `speaker_channel` | `1` | Camera speaker channel |
| `login_timeout` | `5` | AV login timeout in seconds |

The values above are synthetic placeholders. No cloud onboarding is performed
by this source. CAM720 is currently the only verified model; other JOOAN models
may use different protocol versions, authentication, codecs, or speaker
channels.
