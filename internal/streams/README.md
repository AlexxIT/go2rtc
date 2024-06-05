## Testing notes

```yaml
streams:
  test1-basic: ffmpeg:virtual?video#video=h264
  test2-reconnect: ffmpeg:virtual?video&duration=10#video=h264
  test3-execkill: exec:./examples/rtsp_client/rtsp_client/rtsp_client {output}
```
