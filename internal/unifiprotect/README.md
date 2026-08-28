# UniFi Protect cameras

This module receives H.264 and Opus or AAC directly from UniFi Protect cameras,
without a UniFi Protect console or NVR. It implements only the controller
handshake and pushed-media path required for streaming. It has been tested with
a G5 Flex running firmware `4.70.37`.

## Configuration

```yaml
unifi_protect:
  listen: ":7442"
  media_listen: ":7550"

streams:
  entrance_main: unifi-protect://02AABBCCDDEE?channel=video1
  entrance_medium: unifi-protect://02AABBCCDDEE?channel=video2
  entrance_detect: unifi-protect://02AABBCCDDEE?channel=video3&audio=0
```

The source identifier is the camera's 12 hexadecimal MAC digits without
separators; it is case-insensitive. `02AABBCCDDEE` is a locally administered
example address; replace it with the camera's MAC. `channel` defaults to
`video1`; supported values are `video1`, `video2`, and `video3`. Audio is enabled
by default and can be disabled per source with `audio=0`.

All cameras and channels share the two listeners. The WebSocket handshake uses
the TLS listener on `7442`; pushed media uses plain TCP on `7550`. Both ports
must be reachable from the camera. If the host from the camera's WebSocket
request is not a reachable media address, set it explicitly:

```yaml
unifi_protect:
  listen: ":7442"
  media_listen: ":7550"
  media_host: "192.168.1.10"
```

## Camera setup

Set the camera's Protect server URL in its web interface, or run `set-inform`
over SSH after a factory reset:

```text
set-inform 192.168.1.10:7442
```

Use the address of the go2rtc host. Manual `set-inform` is the only adoption
step; this module does not implement UniFi discovery or configure the camera
over SSH.

The controller endpoint is intended for a trusted LAN and has no user
authentication. Do not expose either listener to the Internet.

## TLS identity

The camera pins the exact server certificate when `set-inform` is performed.
When `tls_cert` and `tls_key` are omitted, go2rtc creates
`go2rtc-unifi-protect.crt` and `go2rtc-unifi-protect.key` beside the go2rtc
configuration and reuses them. Back up both files. Deleting or replacing them
disconnects adopted cameras until `set-inform` is run again.

A certificate and key can instead be supplied as paths or inline PEM:

```yaml
unifi_protect:
  listen: ":7442"
  media_listen: ":7550"
  tls_cert: "/config/unifi-protect.crt"
  tls_key: "/config/unifi-protect.key"
```

Normal certificate renewal changes the certificate and therefore also requires
running `set-inform` again. Keeping the same subject or private key is not
sufficient.

## Scope and limitations

- The camera pushes media only while a go2rtc consumer is active.
- Concurrent quality channels use one shared media port and are routed by a
  random per-request token.
- Opus is preferred when advertised by the camera; AAC is used otherwise.
- One producer may be active for each camera/channel pair.
- Camera settings, recording, events, detections, snapshots, talkback, firmware
  management, and UDP discovery are not implemented.
- The control messages do not change microphone enablement, volume, or other
  persistent camera settings.
