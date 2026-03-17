# TP-Link Tapo

[`new in v1.2.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.2.0)

[TP-Link Tapo](https://www.tapo.com/) proprietary camera protocol with **two-way audio** support.

- stream quality is the same as [RTSP protocol](https://www.tapo.com/en/faq/34/)
- use the **cloud password**, this is not the RTSP password! you do not need to add a login!
- you can also use **UPPERCASE** MD5 hash from your cloud password with `admin` username
- some new camera firmwares require SHA256 instead of MD5

## Configuration

```yaml
streams:
  # cloud password without username
  camera1: tapo://cloud-password@192.168.1.123
  # admin username and UPPERCASE MD5 cloud-password hash
  camera2: tapo://admin:UPPERCASE-MD5@192.168.1.123
  # admin username and UPPERCASE SHA256 cloud-password hash
  camera3: tapo://admin:UPPERCASE-SHA256@192.168.1.123
  # VGA stream (the so called substream, the lower resolution one)
  camera4: tapo://cloud-password@192.168.1.123?subtype=1 
  # HD stream (default)
  camera5: tapo://cloud-password@192.168.1.123?subtype=0 
```

```bash
echo -n "cloud password" | md5 | awk '{print toupper($0)}'
echo -n "cloud password" | shasum -a 256 | awk '{print toupper($0)}'
```

## TP-Link Kasa

[`new in v1.7.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)

> [!NOTE]
> This source should be moved to separate module. Because it's source code not related to Tapo.

[TP-Link Kasa](https://www.kasasmart.com/) non-standard protocol [more info](https://medium.com/@hu3vjeen/reverse-engineering-tp-link-kc100-bac4641bf1cd).

- `username` - urlsafe email, `alex@gmail.com` -> `alex%40gmail.com`
- `password` - base64password, `secret1` -> `c2VjcmV0MQ==`

```yaml
streams:
  kc401: kasa://username:password@192.168.1.123:19443/https/stream/mixed
```

Tested: KD110, KC200, KC401, KC420WS, EC71.

## TP-Link Vigi

[`new in v1.9.8`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.8)

[TP-Link VIGI](https://www.vigi.com/) cameras. These are cameras from a different sub-brand, but the format is very similar to Tapo. Only the authorization is different. Read more [here](https://github.com/AlexxIT/go2rtc/issues/1470).

```yaml
streams:
  camera1: vigi://admin:{password}@192.168.1.123
```
