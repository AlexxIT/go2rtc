# Multitrans

**added in v1.9.14** by [@forrestsocool](https://github.com/forrestsocool)

Two-way audio support for Chinese version of [TP-Link cameras](https://www.tp-link.com.cn/list_2549.html).

## Configuration

```yaml
streams:
  tplink_cam:
    # video use standard RTSP
    - rtsp://admin:admin@192.168.1.202:554/stream1
    # two-way audio use MULTITRANS schema
    - multitrans://admin:admin@192.168.1.202:554
```
