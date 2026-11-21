# Xiaomi Mi Home

[`new in v1.9.13`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.9.13)

This source allows you to view cameras from the [Xiaomi Mi Home](https://home.mi.com/) ecosystem.

Since 2020, Xiaomi has introduced a unified protocol for cameras called `miss`. I think it means **Mi Secure Streaming**. Until this point, the camera protocols were in chaos. Almost every model had different authorization, encryption, command lists, and media packet formats.

go2rtc supports two formats: `xiaomi/mess` and `xiaomi/legacy`.
And multiple P2P protocols: `cs2+udp`, `cs2+tcp`, several versions of `tutk+udp`.

Almost all cameras in the `xiaomi/mess` format and the `cs2` protocol work well.
Older `xiaomi/legacy` format cameras may have support issues.
The `tutk` protocol is the worst thing that's ever happened to the P2P world. It works terribly.

**Important:**

1. **Not all cameras are supported**. The list of supported cameras is collected in [this issue](https://github.com/AlexxIT/go2rtc/issues/1982).
2. Each time you connect to the camera, you need Internet access to obtain encryption keys.
3. Connection to the camera is local only, except legacy `tutk` cameras, see [Remote P2P](#remote-p2p-legacy-tutk-cameras).

**Features:**

- Multiple Xiaomi accounts supported
- Cameras from multiple regions are supported for a single account
- Two-way audio is supported
- Cameras with multiple lenses are supported

## Setup

1. Go to go2rtc WebUI > Add > Xiaomi > Login with username and password
2. Receive verification code by email or phone if required.
3. Complete the captcha if required.
4. If everything is OK, your account will be added, and you can load cameras from it.

**Example**

```yaml
xiaomi:
  1234567890: V1:***

streams:
  xiaomi1: xiaomi://1234567890:cn@192.168.1.123?did=9876543210&model=isa.camera.hlc7
```

## Configuration

Quality in the `miss` protocol is specified by a number from 0 to 5. Usually 0 means auto, 1 - sd, 2 - hd.
Go2rtc by default sets quality to 2. But some new cameras have HD quality at number 3.
Old cameras may have broken codec settings at number 3, so this number should not be set for all cameras.

You can change camera quality: `subtype=hd/sd/auto/0-5`.

```yaml
streams:
  xiaomi1: xiaomi://***&subtype=sd
```

You can use a second channel for dual cameras: `channel=2`.

```yaml
streams:
  xiaomi1: xiaomi://***&channel=2
```

## Remote P2P (legacy tutk cameras)

Legacy cameras on the `tutk` protocol can also be reached from outside their LAN, the way Mi Home does it. Use a P2P server instead of the camera's IP; the port **10001** is what selects this mode:

```yaml
streams:
  g2_remote: xiaomi://1234567890:cn@110.43.39.150:10001?did=9876543210&model=lumi.camera.gwagl01
```

go2rtc asks the Xiaomi CN P2P servers where the camera is and asks them to push the camera to punch a hole back, then continues like on the LAN.

- Connecting takes from 1 to about 180 s. The camera registers with a new NAT port after each session, and for a few minutes the server still gives out the old one. Reconnecting within ~60 s, while the previous session is still pinged, takes seconds. Keeping the stream open with [`preload`](../streams/README.md#preload-stream) avoids the wait.
- Verified only with the Aqara G2 (`lumi.camera.gwagl01`) and the Xiaomi CN servers.

## Aqara G2: quality and SD card

The Aqara G2 (`lumi.camera.gwagl01`) accepts these commands in a running session, without reconnecting. They need a running stream of that camera (use `preload` if nobody is watching) and return `404` without it, or `501` for other models.

- `POST /api/xiaomi/quality?did=9876543210&q=fhd|hd|sd` (1920x1080, 1280x720, 640x360) returns when the first frame of the new size has arrived.
- `GET /api/xiaomi/records?did=9876543210&start=<unix>&end=<unix>` returns the recordings on the SD card: `[{"start":1760000000,"dur":60,"type":1}]`.
- `POST /api/xiaomi/playback?did=9876543210&start=<unix>&speed=1|2|4` plays the SD card from that time on the same stream; the response has the name of the file the camera chose. `start=0` goes back to live. `speed` is sent to the camera as is: 1 is normal speed, and the camera then plays about `2 * speed` times faster (2 gives ~4.6x, 4 gives ~8.5x, measured against the on-screen clock); other values are ignored by the camera.
