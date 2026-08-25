# Apple HomeKit

This module supports both client and server for the [Apple HomeKit](https://www.apple.com/home-app/accessories/) protocol.

## HomeKit Client

**Important:**

- You can use HomeKit Cameras **without Apple devices** (iPhone, iPad, etc.), it's just a yet another protocol
- HomeKit device can be paired with only one ecosystem. So, if you have paired it to an iPhone (Apple Home), you can't pair it with Home Assistant or go2rtc. Or if you have paired it to go2rtc, you can't pair it with an iPhone
- HomeKit device should be on the same network with working [mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) between the device and go2rtc

go2rtc supports importing paired HomeKit devices from [Home Assistant](../hass/README.md). 
So you can use HomeKit camera with Home Assistant and go2rtc simultaneously. 
If you are using Home Assistant, I recommend pairing devices with it; it will give you more options.

You can pair device with go2rtc on the HomeKit page. If you can't see your devices, reload the page. 
Also, try rebooting your HomeKit device (power off). If you still can't see it, you have a problem with mDNS.

If you see a device but it does not have a pairing button, it is paired to some ecosystem (Apple Home, Home Assistant, HomeBridge, etc.). You need to delete the device from that ecosystem, and it will be available for pairing. If you cannot unpair the device, you will have to reset it.

**Important:**

- HomeKit audio uses very non-standard **AAC-ELD** codec with very non-standard params and specification violations
- Audio can't be played in `VLC` and probably any other player
- Audio should be transcoded for use with MSE, WebRTC, etc.

### Client Configuration

Recommended settings for using HomeKit Camera with WebRTC, MSE, MP4, RTSP:

```yaml
streams:
  aqara_g3:
    - hass:Camera-Hub-G3-AB12
    - ffmpeg:aqara_g3#audio=aac#audio=opus
```

RTSP link with "normal" audio for any player: `rtsp://192.168.1.123:8554/aqara_g3?video&audio=aac`

**This source is in active development!** Tested only with [Aqara Camera Hub G3](https://www.aqara.com/eu/product/camera-hub-g3) (both EU and CN versions).

## HomeKit Server

[`new in v1.7.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.7.0)

HomeKit module can work in two modes:

- export any H264 camera to Apple HomeKit
- transparent proxy any Apple HomeKit camera (Aqara, Eve, Eufy, etc.) back to Apple HomeKit, so you will have all camera features in Apple Home and also will have RTSP/WebRTC/MP4/etc. from your HomeKit camera

**Important**

- HomeKit cameras support only H264 video and OPUS audio

### Server Configuration

**Minimal config**

```yaml
streams:
  dahua1: rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
homekit:
  dahua1:  # same stream ID from streams list, default PIN - 19550224
```

**Full config**

```yaml
streams:
  dahua1:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0
    - ffmpeg:dahua1#video=h264#hardware  # if your camera doesn't support H264, important for HomeKit
    - ffmpeg:dahua1#audio=opus           # only OPUS audio supported by HomeKit

homekit:
  dahua1:                   # same stream ID from streams list
    pin: 12345678           # custom PIN, default: 19550224
    name: Dahua camera      # custom camera name, default: generated from stream ID
    device_id: dahua1       # custom ID, default: generated from stream ID
    device_private: dahua1  # custom key, default: generated from stream ID
    speaker: true           # enable 2-way audio (default: false, enable only if camera has a speaker)
```

### HKSV (HomeKit Secure Video)

go2rtc can expose any camera as a HomeKit Secure Video (HKSV) camera. This allows Apple Home to record video clips to iCloud when motion is detected.

**Requirements:**
- Apple Home Hub (Apple TV, HomePod or iPad) on the same network
- iCloud storage plan with HomeKit Secure Video support
- Camera source with H264 video (AAC audio recommended)

**No re-pairing needed to enable this on an existing camera.** Some other
guides say you have to remove and re-add an already-paired accessory after
turning on `hksv: true`, because a paired Home Hub only re-reads
`/accessories` when the mDNS config number (`c#`) changes, and hardcoding it
to `1` means it never does. This fork derives `c#` from a hash of the
accessory database instead, so it moves whenever the database does -
including the moment you flip on `hksv: true` - and the hub picks up the new
recording services on its own.

**Minimal HKSV config**

```yaml
streams:
  outdoor: rtsp://admin:password@192.168.1.123/stream1

homekit:
  outdoor:
    hksv: true           # enable HomeKit Secure Video
    motion: continuous   # always report motion, Home Hub decides what to record
```

**Full HKSV config**

```yaml
streams:
  outdoor:
    - rtsp://admin:password@192.168.1.123/stream1
    - ffmpeg:outdoor#video=h264#hardware  # transcode to H264 if needed
    - ffmpeg:outdoor#audio=aac            # AAC-LC audio for HKSV recording

homekit:
  outdoor:
    pin: 12345678
    name: Outdoor Camera
    hksv: true
    motion: api          # motion triggered via API
```

**Non-16:9 sensors (portrait doorbells, unusual aspect ratios)**

By default the accessory advertises the standard 1920x1080 and 1280x720
landscape configurations. A camera whose sensor isn't 16:9 - a portrait
doorbell, most commonly - needs its own:

```yaml
homekit:
  front_door:
    hksv: true
    recording_resolution: 1200x1600  # match your sensor's aspect ratio
```

This only changes what the accessory *advertises* - it does not resize,
crop, or transcode anything. The video source is untouched either way.

That distinction matters because of what's actually been observed against a
real Home Hub: HKSV tolerates recording a **larger landscape** resolution
than advertised without complaint (a 4K source against a 1920x1080/1280x720
advertised config records fine, no transcode needed) - but a **portrait**
source against only-landscape advertised configs stalls silently. The hub
picks a Selected Configuration and then simply never opens a DataStream, no
error anywhere. So `recording_resolution` is usually only necessary to fix
the *orientation* mismatch, not the size - set it to any portrait size, even
one much smaller than your actual sensor, and passthrough of the real
(larger) resolution still works.

**HKSV Doorbell config**

```yaml
homekit:
  front_door:
    category_id: doorbell
    hksv: true
    motion: api
```

**Motion modes:**

- `continuous` — MotionDetected is always true; Home Hub continuously receives video and decides what to save. Simplest setup, but see the warning below - clips from a static scene get uploaded and then silently discarded, which looks exactly like broken recording.
- `detect` — automatic motion detection by analyzing H264 P-frame sizes. No external dependencies or CPU-heavy decoding. Works with any H264 source and resolution. Compares each P-frame size against an adaptive baseline using EMA (exponential moving average). When a P-frame exceeds the threshold ratio, motion is triggered with a 30s hold time and 5s cooldown.
- `onvif` — subscribes to the camera's own ONVIF PullPoint motion events, so detection happens in-camera instead of being inferred from the video. Usually the most accurate option and costs nothing extra, when the camera supports it.
- `api` — motion is triggered externally via HTTP API. Use this with Frigate, an ONVIF bridge you already run, or any other motion detection system.

**Warning: with `continuous`, "no recordings appear" can mean nothing was actually
recorded, not that something is broken.** Apple analyzes every clip HKSV uploads,
and one showing a static, empty scene gets discarded - silently, with zero error
on either the accessory or hub side. Fragments still flush, the DataStream still
opens and closes cleanly, the whole pipeline looks completely healthy throughout.
Before debugging "recording doesn't work," confirm something was actually moving
in front of the camera during the window you're checking. `detect`, `onvif` and
`api` don't have this problem, since they only trigger on real activity.

**Motion detect config:**

```yaml
homekit:
  outdoor:
    hksv: true
    motion: detect
    motion_threshold: 1.0  # P-frame size / baseline ratio to trigger motion (default: 2.0)
```

The `motion_threshold` controls sensitivity — it's the ratio of P-frame size to the adaptive baseline. When a P-frame exceeds `baseline × threshold`, motion is triggered.

| Scenario | threshold | Notes |
|---|---|---|
| Quiet indoor scene | 1.3–1.5 | Low noise, stable baseline, even small motion is visible |
| Standard camera (yard, hallway) | 2.0 (default) | Good balance between sensitivity and false positives |
| Outdoor with trees/shadows/wind | 2.5–3.0 | Wind and shadows produce medium P-frames, need margin |
| Busy street / complex scene | 3.0–5.0 | Lots of background motion, react only to large events |

Values below 1.0 are meaningless (triggers on every frame). Values above 5.0 require very large motion (person filling half the frame).

**How to tune:** set `log.level: trace` and watch `motion: status` lines — they show current `ratio`. Walk in front of the camera and note the ratio values:

```
motion: status baseline=5000 ratio=0.95  ← quiet
motion: status baseline=5000 ratio=3.21  ← person walked by
motion: status baseline=5000 ratio=1.40  ← shadow/wind
```

Set threshold between "noise" and "real motion". In this example, 2.0 is a good choice (ignores 1.4, catches 3.2).

The table above is a starting point, not a substitute for tuning per camera from
real data - noise floors vary far more between individual cameras than by scene
type. Two failure modes worth knowing about, since no threshold fixes either one:

- **A flat noise floor**: some cameras (a reflective surface, a display, IR
  interference) sit near the trigger line constantly, producing dozens of
  false triggers an hour with no time-of-day pattern. Only a much higher
  threshold helps here, and it's worth checking *why* the noise floor is
  elevated rather than just raising the number.
- **A dawn/dusk ramp**: as a scene brightens or dims, P-frame size can climb
  or fall over several minutes faster than the EMA baseline (`motionAlphaSlow`)
  can track it, causing a sustained run of false triggers - not discrete
  events, a steadily climbing ratio across many consecutive triggers in one
  window. A threshold high enough to survive this is too high for real
  detection the rest of the day. `onvif` or `api` avoid this failure mode
  entirely, since they don't infer motion from frame size at all.

**Motion via ONVIF:**

```yaml
homekit:
  outdoor:
    hksv: true
    motion: onvif
    onvif_url: onvif://user:pass@192.168.1.123:80  # optional - auto-discovered
                                                     # from the stream's sources
                                                     # if omitted
    motion_hold_time: 30  # seconds motion stays "true" after the last event
                           # (default: 30)
```

Subscribes to the camera's ONVIF PullPoint event service and forwards real
motion events, instead of inferring them from the video. Requires the camera's
ONVIF *credentials* specifically - some cameras (Axis, notably) use a separate
ONVIF user configured in the camera's own settings, distinct from the admin
login used for RTSP and the web UI, so the same password that works everywhere
else may still get rejected here with a SOAP `NotAuthorized` fault. Also
requires an `http://` ONVIF service - an HTTPS-only camera web interface isn't
supported by the ONVIF client yet.

**Motion API:**

```bash
# Get motion status
curl "http://localhost:1984/api/homekit/motion?id=outdoor"
# → {"id":"outdoor","motion":false}

# Trigger motion start
curl -X POST "http://localhost:1984/api/homekit/motion?id=outdoor"

# Clear motion
curl -X DELETE "http://localhost:1984/api/homekit/motion?id=outdoor"

# Trigger doorbell ring
curl -X POST "http://localhost:1984/api/homekit/doorbell?id=front_door"

# Dump the exact fMP4 (init segment + fragments) the HKSV consumer would send
# to the controller - useful for checking what a clip actually looks like when
# something is being delivered but not appearing as a recording
curl "http://localhost:1984/api/homekit/recording.mp4?src=outdoor&wait=15" -o clip.mp4
```

**Proxy HomeKit camera**

- Video stream from HomeKit camera to Apple device (iPhone, Apple TV) will be transmitted directly
- Video stream from HomeKit camera to RTSP/WebRTC/MP4/etc. will be transmitted via go2rtc

```yaml
streams:
  aqara1:
    - homekit://...
    - ffmpeg:aqara1#audio=aac#audio=opus  # optional audio transcoding

homekit:
  aqara1:  # same stream ID from streams list
```
