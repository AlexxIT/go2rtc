# Bubble

[`new in v1.6.1`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.6.1)

Private format in some cameras from [dvr163.com](http://help.dvr163.com/) and [eseecloud.com](http://www.eseecloud.com/).

## Configuration

- you can skip `username`, `password`, `port`, `ch` and `stream` if they are default
- set up separate streams for different channels and streams

```yaml
streams:
  camera1: bubble://username:password@192.168.1.123:34567/bubble/live?ch=0&stream=0
```
