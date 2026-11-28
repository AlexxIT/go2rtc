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

The ONVIF module does not support Basic authentication. Therefore `/onvif/*` will use the `username` and `password` from the RTSP module. If it is not specified, it will use `username` and `password` from the API module.

ONVIF clients receive a video stream from the RTSP module and snapshots from the API module. And they will use the same username and password that you provided to connect to ONVIF.

### ONVIF Events

The ONVIF server has an Events service (PullPoint subscriptions only) with motion events. Clients like UniFi Protect use it to show and record motion from third-party cameras. Both common motion topics are sent: `tns1:RuleEngine/CellMotionDetector/Motion` (`IsMotion`) and `tns1:VideoSource/MotionAlarm` (`State`). The event source token is the stream name.

Events come from the [motion state](../streams/README.md#motion-state) of streams, which any external detector can set with the API:

```shell
curl -X POST "http://localhost:1984/api/motion?src=camera1"   # motion started
curl -X DELETE "http://localhost:1984/api/motion?src=camera1" # motion ended
```

- the Events service is always advertised in `GetCapabilities` and `GetServices`
- a new subscription gets one `Initialized` message per stream with a known motion state, streams without a state send nothing until their first change

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
