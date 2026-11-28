# Easy4IP (Dahua P2P)

Cameras and recorders (NVR/XVR) of Dahua and its brands (KBVision, Imou, ...) that DMSS can reach from outside the LAN through Dahua's Easy4IP P2P cloud. go2rtc speaks the same P2P protocol, so no port forwarding is needed: you only need the device's **serial number** and its login.

The source opens one P2P session per device and tunnels the device's RTSP (and optionally HTTP) port through it, then hands the stream to the RTSP source.

```yaml
streams:
  nvr_ch1_main: easy4ip://admin:password@9A0123456789ABC/cam/realmonitor?channel=1&subtype=0
  nvr_ch1_sub:  easy4ip://admin:password@9A0123456789ABC/cam/realmonitor?channel=1&subtype=1
```

All streams of one serial share the session. Options in the URL (they are removed before the RTSP request goes to the device):

| option        | default                | meaning                                                                    |
|---------------|------------------------|----------------------------------------------------------------------------|
| `rtsp_port`   | 554                    | RTSP port of the device                                                    |
| `http_port`   | not tunnelled          | also tunnel this device port (80) for CGI calls, see `/api/easy4ip`           |
| `client_id`   | stable per host+serial | `<uuid>:37777`, DMSS sends its install UUID here                           |
| `relay`       | off                    | `relay=1` carries the whole session through the Dahua relay (experimental) |

Any other query parameter and the `#fragment` go to the RTSP source as they are, for example `#backchannel=0`.

`GET /api/easy4ip` lists the sessions: `[{"serial":"...","up":true,"ports":{"554":41234,"80":41235}}]`. Those loopback ports reach the device ports, e.g. `curl --digest -u admin:password "http://127.0.0.1:41235/cgi-bin/mediaFileFind.cgi?action=factory.create"`.

## Limits

- **One P2P session per device and client.** Do not run DMSS, another go2rtc or the Rust `dh-p2p` tunnel against the same device at the same time: a second session hangs, and while one is live the device may drop frames.
- **Bandwidth.** A session carries about 4-5 Mbps in total. That is one main stream (~2 Mbps) plus a few sub streams; several main streams at once make the NVR drop frames, and with many it can fall off the P2P cloud for a while.
- **Restarts take about 30 s.** The handshake opens a relay leg that the device keeps for ~30 s and that cannot be closed early. A new session started inside that window waits for it, so after a restart expect one 30 s timeout before it connects. On a clean shutdown go2rtc sends the device an end-of-session command (the same as DMSS), which avoids most of this. Use `preload` to keep the session open.
- A device clock that disagrees with ours by more than a few minutes gets `TimeOut` errors on the first try; the offset is learnt from the reply and the retry works.
- Only devices that need no per-device salt in the login are supported (the `randsalt` firmwares are not).

## Credits

The protocol is from [khoanguyen-3fc/dh-p2p](https://github.com/khoanguyen-3fc/dh-p2p) (MIT), with fixes found against a real XVR.
