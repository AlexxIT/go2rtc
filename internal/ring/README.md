# Ring

[`new in v1.9.13`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13) by [@seydx](https://github.com/seydx)

This source type supports Ring cameras with [two-way audio](../../README.md#two-way-audio) support.

## Configuration

If you have a `refresh_token` and `device_id`, you can use them in the `go2rtc.yaml` config file.

Otherwise, you can use the go2rtc web interface and add your Ring account (WebUI > Add > Ring). Once added, it will list all your Ring cameras.

```yaml
streams:
  ring: ring:?device_id=XXX&refresh_token=XXX
  ring_snapshot: ring:?device_id=XXX&refresh_token=XXX&snapshot
```
