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

### ONVIF Events

The ONVIF server has an Events service (PullPoint subscriptions only) with motion events. Clients like UniFi Protect use it to show and record motion from third-party cameras. Both common motion topics are sent: `tns1:RuleEngine/CellMotionDetector/Motion` (`IsMotion`) and `tns1:VideoSource/MotionAlarm` (`State`). The event source token is the stream name.

Go2rtc doesn't detect motion itself. You can set the motion state of a stream from any external detector with the API:

```shell
curl -X POST "http://localhost:1984/api/onvif/motion?src=camera1&state=true"
curl -X POST "http://localhost:1984/api/onvif/motion?src=camera1&state=false"
```

- `state` - `true` or `false` (also `1`/`0`, `on`/`off`), send `false` to end the event
- the stream becomes a motion source with the first API call
- the Events service is advertised in `GetCapabilities` and `GetServices` only when at least one motion source exists, so call the API (ex. with `state=false`) before adding the camera to the client
- motion states live in memory and are lost after a restart

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
