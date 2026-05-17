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

  # Standard-definition (H.264) stream with channel 0
  driveway_sub:
    - reolink://[user name]:[password]@192.168.1.123/sub?channel=0
    - ffmpeg:driveway_sub#audio=opus
```

## Options & Parameters

| Query Parameter | Default Value | Description |
|-----------------|---------------|-------------|
| `channel`       | `0`           | The camera video channel/lens number (starts at `0` for single-lens and multi-lens cameras). |

## Two-Way Audio (Talkback)

When you click the microphone button in Frigate or the HA WebUI, the browser initiates a WebRTC connection. Since WebRTC strictly only supports **Opus** audio, we recommend wrapping the `reolink` stream with `ffmpeg:...#audio=opus` as shown above. This ensures:
1. One-way playback uses the camera's raw `AAC` or `PCM` stream via MSE (0% CPU usage).
2. Clicking the microphone dynamically handles both the incoming microphone track resampled/transcoded to the camera speaker, and transcoding the camera's native audio to Opus.

## Idle Connection Management

By default, `go2rtc` only dials and connects to your Reolink cameras when a client is actively watching. When the last client closes the stream, `go2rtc` waits 30 seconds (idle timeout) and then completely tears down the TCP connection to the camera, freeing up all of the camera's internal sockets and CPU resources.
