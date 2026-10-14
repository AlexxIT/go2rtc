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

## Two-way audio and playback

Tapo cameras support only one talk session. The audio is sent as PCMA/8000.

Playback of files, TTS or radio streams with FFmpeg ([stream to camera](../../README.md#stream-to-camera))
needs the paced mode for battery powered cameras and doorbells (tested with D235). FFmpeg delivers the audio in
packets of the source frame size and in bursts. The camera stutters if they are forwarded as is.

```yaml
streams:
  # video and two-way audio from a browser, lowest latency
  doorbell: tapo://cloud-password@192.168.1.123
  # playback only: POST /api/streams?dst=doorbell_playback&src=...
  doorbell_playback: tapo://cloud-password@192.168.1.123?backchannel_pace=120&backchannel_prefill=400&backchannel_leadin=1500&backchannel_wake=6000
```

| Param                 | Range   | Default | Description                                                                                                                                              |
|-----------------------|---------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `backchannel_pace`    | 0..2000 | off     | Enables the paced mode: the audio is queued, cut into chunks of exactly 40 ms and sent on a 40 ms clock. Value: ms of audio collected before the start. |
| `backchannel_prefill` | 0..2000 | 80      | Silence in ms sent at once before the audio. It is the cushion of the camera for the jitter of the network.                                              |
| `backchannel_leadin`  | 0..5000 | 0       | Paced mode only: silence in ms sent in real time before the audio. A doorbell needs a moment to switch on the speaker, short messages are lost without. |
| `backchannel_wake`    | 0..15000 | 0      | Paced mode only: ms between the wake up of a battery camera and the start of the talk session, the audio waits. See below.                                |

A D235 on battery freezes for 2-3 seconds about 4 seconds after it was woken up, if a talk session is active at this
time: it sends no video and reads no audio, the playback stutters. A talk session that starts 5.5 seconds or later
after the wake up is not affected. With `backchannel_wake=6000` the talk session is started only when go2rtc is
connected to the camera for 6 seconds (other streams of the same camera count, also a few seconds after they were closed). Until then
the audio is queued. A camera with a running stream gets the audio at once.

Use it only while the camera runs on battery. go2rtc can't tell whether a camera on mains power is awake, so every
playback that is not started within a few seconds after another connection is delayed for nothing.

In the paced mode:

- Several playbacks at the same time are mixed into one talk session. A new playback doesn't stop the running one.
  Without the paced mode (and for other cameras) a new playback replaces the running one.
- An empty source (`POST /api/streams?dst=doorbell&src=`) stops all playbacks at once.
- The audio is delayed by `backchannel_pace` and `backchannel_leadin`. This is why the example has a separate stream
  for the playback: two-way audio from a browser to the same stream would have the same delay.
- Two-way audio from a browser and a playback at the same time don't work with separate streams, the camera rejects
  the second talk session.

Set `log: {tapo: debug}` to see the statistics of every playback: sent chunks, mixed chunks, underruns and the timing
of the sender.

## Codecs

Battery powered cameras and doorbells (ex. D235 on battery) send H265 video and Opus audio over this protocol, other
cameras send H264 and PCMA. The codecs are detected from the first packets when the connection is opened.

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
