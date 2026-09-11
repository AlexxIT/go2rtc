# ONVIF

## ONVIF Client

[`new in v1.5.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)

The source is not very useful if you already know RTSP and snapshot links for your camera. But it can be useful if you don't.

**WebUI > Add** webpage supports ONVIF autodiscovery. Your server must be on the same subnet as the camera. If you use Docker, you must use "network host".

```yaml
streams:
  dahua1: onvif://admin:password@192.168.1.123
  reolink1: onvif://admin:password@192.168.1.123:8000
  tapo1: onvif://admin:password@192.168.1.123:2020
```

## ONVIF Server

A regular camera has a single video source (`GetVideoSources`) and two profiles (`GetProfiles`).

Go2rtc has one video source and one profile per stream.

### Security: authenticating ONVIF server requests

The ONVIF server built into go2rtc can verify the standard ONVIF
authentication mechanism — `wsse:UsernameToken` (PasswordDigest)
carried in the SOAP `Security` header — using dedicated
`onvif.username` / `onvif.password` config keys, separate from
the HTTP Basic auth used by the WebUI / JSON API (`api.username`
/ `api.password`), since real ONVIF clients speak WS-Security,
not HTTP Basic.

```yaml
onvif:
  username: "admin"  # default "", enables WS-Security UsernameToken checks
  password: "pass"   # default ""
```

When set, every ONVIF operation except `GetSystemDateAndTime`
requires a valid, freshly-timestamped `UsernameToken` matching
these credentials; unauthenticated or mismatched requests get a
SOAP fault with HTTP 401. `GetSystemDateAndTime` stays open so
clients can learn the server's clock before computing a digest
against it. Leave `onvif.username` unset (the default) to keep
the ONVIF server open, e.g. for trusted-network-only setups.

## Tested clients

Go2rtc works as ONVIF server:

- Happytime onvif client (windows)
- Home Assistant ONVIF integration (linux)
- Onvier (android)
- ONVIF Device Manager (windows)

PS. Supports only TCP transport for RTSP protocol. UDP and HTTP transports - unsupported yet.

## Tested cameras

Go2rtc works as ONVIF client:

- Dahua IPC-K42
- OpenIPC
- Reolink RLC-520A
- TP-Link Tapo TC60
