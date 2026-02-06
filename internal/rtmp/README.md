# Real-Time Messaging Protocol

This module provides the following features for the RTMP protocol:

- Streaming input - [RTMP client](#rtmp-client)
- Streaming output and ingest in `rtmp` format - [RTMP server](#rtmp-server)
- Streaming output and ingest in `flv` format - [FLV server](#flv-server)

## RTMP Client

You can get a stream from an RTMP server, for example [Nginx with nginx-rtmp-module](https://github.com/arut/nginx-rtmp-module).

### Client Configuration

```yaml
streams:
  rtmp_stream: rtmp://192.168.1.123/live/camera1
```

## RTMP Server

[`new in v1.8.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.0)

Streaming output stream in `rtmp` format:

```shell
ffplay rtmp://localhost:1935/camera1
```

Streaming ingest stream in `rtmp` format:

```shell
ffmpeg -re -i BigBuckBunny.mp4 -c copy -f flv rtmp://localhost:1935/camera1
```

### Server Configuration

By default, the RTMP server is disabled.

```yaml
rtmp:
  listen: ":1935"  # by default - disabled!
```

## FLV Server

Streaming output in `flv` format.

```shell
ffplay http://localhost:1984/stream.flv?src=camera1
```

Streaming ingest in `flv` format.

```shell
ffmpeg -re -i BigBuckBunny.mp4 -c copy -f flv http://localhost:1984/api/stream.flv?dst=camera1
```

## Tested client

| From   | To                              | Comment |
|--------|---------------------------------|---------|
| go2rtc | Reolink RLC-520A fw. v3.1.0.801 | OK      |

**go2rtc.yaml**

```yaml
streams:
  rtmp-reolink1: rtmp://192.168.10.92/bcs/channel0_main.bcs?channel=0&stream=0&user=admin&password=password
  rtmp-reolink2: rtmp://192.168.10.92/bcs/channel0_sub.bcs?channel=0&stream=1&user=admin&password=password
  rtmp-reolink3: rtmp://192.168.10.92/bcs/channel0_ext.bcs?channel=0&stream=1&user=admin&password=password
```

## Tested server

| From                   | To     | Comment             |
|------------------------|--------|---------------------|
| OBS 31.0.2             | go2rtc | OK                  |
| OpenIPC 2.5.03.02-lite | go2rtc | OK                  |
| FFmpeg 6.1             | go2rtc | OK                  |
| GoPro Black 12         | go2rtc | OK, 1080p, 5000kbps |

**go2rtc.yaml**

```yaml
rtmp:
  listen: :1935
streams:
  tmp:
```

**OBS**
 
Settings > Stream:

- Service: Custom
- Server: rtmp://192.168.10.101/tmp
- Stream Key: `<empty>`
- Use auth: `<disabled>`

**OpenIPC**

WebUI > Majestic > Settings > Outgoing

- Enable
- Address: rtmp://192.168.10.101/tmp
- Save
- Restart

**FFmpeg**

```shell
ffmpeg -re -i bbb.mp4 -c copy -f flv rtmp://192.168.10.101/tmp
```

**GoPro**

GoPro Quik > Camera > Translation > Other
