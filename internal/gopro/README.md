# GoPro

Supported models: HERO9, HERO10, HERO11, HERO12.  
Supported OS: Linux, Mac, Windows, [HassOS](https://www.home-assistant.io/installation/)

The other camera models have different APIs. I will try to add them in the next versions.

## Config

- USB-connected cameras create a new network interface in the system
- Linux users do not need to install anything
- Windows users should install the [network driver](https://community.gopro.com/s/article/GoPro-Webcam)
- if the camera is detected but the stream does not start - you need to disable firewall

1. Discover camera address: WebUI > Add > GoPro
2. Add camera to config

```yaml
streams:
  hero12: gopro://172.20.100.51
```

## Useful links

- https://gopro.github.io/OpenGoPro/
