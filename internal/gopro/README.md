# GoPro

[`new in v1.8.3`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.8.3)

Support streaming from [GoPro](https://gopro.com/) cameras, connected via USB or Wi-Fi to Linux, Mac, Windows.

Supported models: HERO9, HERO10, HERO11, HERO12.  
Supported OS: Linux, Mac, Windows, [HassOS](https://www.home-assistant.io/installation/)

Other camera models have different APIs. I will try to add them in future versions.

## Configuration

- USB-connected cameras create a new network interface in the system
- Linux users do not need to install anything
- Windows users should install the [network driver](https://community.gopro.com/s/article/GoPro-Webcam)
- if the camera is detected but the stream does not start, you need to disable the firewall

1. Discover camera address: WebUI > Add > GoPro
2. Add camera to config

```yaml
streams:
  hero12: gopro://172.20.100.51
```

## Useful links

- https://gopro.github.io/OpenGoPro/
