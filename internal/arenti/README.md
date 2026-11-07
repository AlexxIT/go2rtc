# Arenti & Meari

This source allows you to stream from [Arenti](https://www.arenti.com/) and [Meari](https://www.meari.com/) cloud cameras using native WebRTC (MTS protocol) directly to go2rtc.

**Features:**
- Native WebRTC P2P streaming with COTURN cloud signaling
- H.264 video and PCMA / PCM / AAC audio support
- Automatic camera wake-up for battery-powered cameras
- On-demand streaming (battery saving) and always-connected preload for AC/wired cameras
- Stream quality selection: HD (default) and SD via `stream`, `subtype`, or `resolution`
- Multi-account support
- Camera auto-discovery via go2rtc WebUI

## Setup

1. Add your Arenti / Meari account credentials to `go2rtc.yaml`:

```yaml
arenti:
  username: "user@example.com"
  password: "your-password"
  country_code: "DK"          # ISO 2-letter country code (default: US)

streams:
  living_room: arenti://aa2d2bc5598e4e4c
```

Or configure multiple accounts:

```yaml
arenti:
  user1@example.com:
    password: "password1"
    country_code: "DK"
  user2@example.com:
    password: "password2"
    country_code: "US"

streams:
  cam1: arenti://aa2d2bc5598e4e4c?account=user1%40example.com
```

## Stream Quality & Parameters

By default, go2rtc requests the **HD** (High Definition) stream. You can customize the stream using standard query parameters:

| Parameter | Values | Description |
|-----------|--------|-------------|
| `stream` | `hd` (default), `sd`, `main`, `sub`, `1`, `0` | Selects HD (`1`) or SD (`0`) quality |
| `subtype` | `0` / `hd` (default: HD), `1` / `sd` (SD) | Standard go2rtc subtype alias |
| `resolution` | `hd` (default), `sd`, `main`, `sub` | Resolution alias (like Tuya) |
| `channel` | `0` (default), `1`, ... | Video channel number |
| `battery` | `true` (default), `false` | Battery optimization mode |

### Quality Examples

```yaml
streams:
  # HD quality (default - no parameters needed)
  boat_cam_hd: arenti://aa2d2bc5598e4e4c

  # Explicit HD via subtype (go2rtc standard)
  boat_cam_hd2: arenti://aa2d2bc5598e4e4c?subtype=0

  # SD / sub-stream via stream parameter
  boat_cam_sd: arenti://aa2d2bc5598e4e4c?stream=sd

  # SD / sub-stream via subtype parameter
  boat_cam_sd2: arenti://aa2d2bc5598e4e4c?subtype=1

  # Wired / AC camera (keeps stream preloaded, disables battery sleep)
  front_door_wired: arenti://aa2d2bc5598e4e4c?battery=no
```
