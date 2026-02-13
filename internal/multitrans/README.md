# TP-Link MULTITRANS

[`new in v1.9.14`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.14) by [@forrestsocool](https://github.com/forrestsocool)

Two-way audio support for Chinese version of [TP-Link](https://www.tp-link.com.cn/) cameras.

## Configuration

```yaml
streams:
  tplink_cam:
    # video uses standard RTSP
    - rtsp://admin:admin@192.168.1.202:554/stream1
    # two-way audio uses MULTITRANS schema
    - multitrans://admin:admin@192.168.1.202:554
```

## Useful links

- https://www.tp-link.com.cn/list_2549.html
- https://github.com/AlexxIT/go2rtc/issues/1724
- https://github.com/bingooo/hass-tplink-ipc/
