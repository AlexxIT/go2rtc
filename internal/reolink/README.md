# Reolink Native Baichuan Protocol

[`new in v1.10.0`](https://github.com/AlexxIT/go2rtc/releases/latest)

Reolink proprietary camera protocol (Baichuan XML/ONVIF) with native **two-way audio (talkback)** and high-performance video streaming.

- **Zero configuration required**: Auto-discovers and negotiates the camera's two-way audio talk profiles directly.
- **Ultra-low latency**: Low-overhead native Baichuan TCP connections for optimal live view.
- **Out-of-order B-frame preservation**: Corrects timestamps dynamically on the fly to eliminate compression artifacts and trails.
- **Automated Resource Management**: Automatically disconnects from the camera after 30 seconds of client inactivity to save camera CPU and system bandwidth.

## Configuration

```yaml
streams:
  # High-definition (H.265/H.264) stream with channel 0 (default)
  driveway_main:
    - reolink://[user name]:[password]@192.168.1.123/main?channel=0
    # Enable WebRTC two-way talkback audio support:
    - ffmpeg:driveway_main#audio=opus

  # High-definition stream using the camera's local UID (P2P/UDP broadcast)
  driveway_p2p:
    - reolink://[user name]:[password]@9527000000000000/main?channel=0
    - ffmpeg:driveway_p2p#audio=opus

  # Standard-definition (H.264) stream with channel 0
  driveway_sub:
    - reolink://[user name]:[password]@192.168.1.123/sub?channel=0
    - ffmpeg:driveway_sub#audio=opus
```

## Options & Parameters

| Query Parameter | Default Value | Description |
|-----------------|---------------|-------------|
| `channel`       | `0`           | The camera video channel/lens number (starts at `0` for single-lens and multi-lens cameras). |
| `video`         | `true`        | Set to `false` to completely disable the video track (useful to prevent resolution switching during talkback). |
| `audio`         | `true`        | Set to `false` to completely disable the incoming audio track. |

## Two-Way Audio (Talkback)

When you click the microphone button in Frigate or the HA WebUI, the browser initiates a WebRTC connection. Since WebRTC strictly only supports **Opus** audio, we recommend wrapping the `reolink` stream with `ffmpeg:...#audio=opus` as shown above. This ensures:
1. One-way playback uses the camera's raw `AAC` or `PCM` stream via MSE (0% CPU usage).
2. Clicking the microphone dynamically handles both the incoming microphone track resampled/transcoded to the camera speaker, and transcoding the camera's native audio to Opus.

## Main Stream Talkback & H.265 WebRTC Codec Issues

Reolink firmware has a limitation: they cannot handle video output and talkback audio over the same `main` stream connection simultaneously (initiating talkback on `main` causes the camera to drop or freeze the video). However, the camera can perfectly output high-resolution `main` stream video while talkback is handled over a separate `sub` or `extern` connection.

To support talkback on your main high-res live view, configure the `main` and `sub`/`extern` streams together as alternative sources:

### Option A: Automatic Low-Res Fallback (Universal / Compatible)
If your browser (like Microsoft Edge or older Chrome/Firefox versions) does not support H.265 (HEVC) over WebRTC:
```yaml
streams:
  doorbell_1:
    - reolink://admin:password@192.168.1.123/main
    - reolink://admin:password@192.168.1.123/extern
    - ffmpeg:doorbell_1#audio=opus
```
* **How it works:** Under normal live viewing, the player uses MSE to stream high-resolution H.265 video. When you click the microphone button, the player switches to WebRTC. Since your browser doesn't support H.265 WebRTC, go2rtc automatically falls back to streaming H.264 video from the `extern`/`sub` stream. When you turn off the mic, the player switches back to high-res H.265 MSE automatically.

### Option B: High-Res H.265 WebRTC (Forces High-Resolution Video)
If your browser supports H.265 over WebRTC, you can prevent go2rtc from falling back to low-resolution video by disabling the video track on the secondary stream:
```yaml
streams:
  doorbell_1:
    - reolink://admin:password@192.168.1.123/main
    - reolink://admin:password@192.168.1.123/extern?video=false
    - ffmpeg:doorbell_1#audio=opus
```
* **How it works:** By appending `?video=false`, the `extern`/`sub` stream is used purely as an audio/talkback backchannel. Go2rtc is forced to negotiate video exclusively from the `main` stream under WebRTC.

#### Browser Compatibility for H.265 WebRTC:
- **Google Chrome**: Supported by default in modern versions (Chrome 136+) on Windows, macOS, and Android, provided graphics hardware acceleration is enabled. On Windows, you also need the official **HEVC Video Extensions** installed from the Microsoft Store.
- **Safari**: Supported natively on macOS and iOS.
- **Microsoft Edge**: **Not supported.** Even though Edge is based on Chromium, it does not support WebRTC H.265/HEVC decoding. If you use Edge with Option B, the video will fail to load when you click the microphone. Use Option A or switch to Chrome.

## Idle Connection Management

By default, `go2rtc` only dials and connects to your Reolink cameras when a client is actively watching. When the last client closes the stream, `go2rtc` waits 30 seconds (idle timeout) and then completely tears down the TCP connection to the camera, freeing up all of the camera's internal sockets and CPU resources.

## Local P2P/UID Connections

If your camera's IP address changes frequently (dynamic DHCP) or if you want to route local traffic purely via the camera's **16-character unique identifier (UID)**, the native `reolink` driver supports automatic local UDP broadcast discovery:

1. Specify the camera's UID in place of the hostname (e.g. `reolink://admin:password@9527000000000000/main`).
2. When the stream is requested, the driver broadcasts local UDP discovery packets on ports `2015` and `2018`.
3. The camera automatically answers, and a highly efficient, reliable P2P UDP transport layer is established on your LAN completely bypassing static IP configurations!

> [!NOTE]
> This is a local-only feature. WAN/Remote P2P (connecting across firewalls when the camera is not on the same LAN) is not supported to ensure your IoT data is kept strictly inside your own private network.
