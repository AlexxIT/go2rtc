# TP-Link Kasa

[`new in v1.7.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)

[TP-Link Kasa](https://www.kasasmart.com/) non-standard protocol [more info](https://medium.com/@hu3vjeen/reverse-engineering-tp-link-kc100-bac4641bf1cd).

- `username` - urlsafe email, `alex@gmail.com` -> `alex%40gmail.com`
- `password` - base64password, `secret1` -> `c2VjcmV0MQ==`

```yaml
streams:
  kc401: kasa://username:password@192.168.1.123:19443/https/stream/mixed
```

Tested: KD110, KC200, KC401, KC420WS, EC71.
