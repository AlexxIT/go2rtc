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
- Stream Key: <empty>
- Use auth: <disabled>

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
