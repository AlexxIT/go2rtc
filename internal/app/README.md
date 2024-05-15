- By default go2rtc will search config file `go2rtc.yaml` in current work directory
- go2rtc support multiple config files
- go2rtc support inline config as `YAML`, `JSON` or `key=value` format from command line
- Every next config will overwrite previous (but only defined params)

```
go2rtc -config "{log: {format: text}}" -config /config/go2rtc.yaml -config "{rtsp: {listen: ''}}" -config /usr/local/go2rtc/go2rtc.yaml
```

or simple version

```
go2rtc -c log.format=text -c /config/go2rtc.yaml -c rtsp.listen='' -c /usr/local/go2rtc/go2rtc.yaml
```

## Environment variables

Also go2rtc support templates for using environment variables in any part of config:

```yaml
streams:
  camera1: rtsp://rtsp:${CAMERA_PASSWORD}@192.168.1.123/av_stream/ch0

  ${LOGS:}  # empty default value

rtsp:
  username: ${RTSP_USER:admin}   # "admin" if env "RTSP_USER" not set
  password: ${RTSP_PASS:secret}  # "secret" if env "RTSP_PASS" not set
```

## Defaults

```yaml
api:
  listen: ":1984"

ffmpeg:
  bin: "ffmpeg"

log:
  level: "info"

rtsp:
  listen: ":8554"
  default_query: "video&audio"

srtp:
  listen: ":8443"

webrtc:
  listen: ":8555/tcp"
  ice_servers:
    - urls: [ "stun:stun.l.google.com:19302" ]
```