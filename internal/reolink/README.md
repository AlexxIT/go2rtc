# Reolink

Experimental native Baichuan source for Reolink cameras on a trusted local network.

- H.264/H.265 video from `main`, `sub`, and `extern` profiles
- AAC audio or camera ADPCM converted to PCMA
- PCMA/PCMU/PCM/PCML two-way audio converted to camera ADPCM
- Direct LAN TCP or UID-discovered LAN-P2P over reliable UDP
- Explicit NVR and multi-lens channels

No FFmpeg, RTSP, or HTTP-FLV input bridge is required.

## Compatibility

The adapter discovers the selected profile's codecs from live media and probes talkback directly. Model identity, observed channels, and capability-query status are diagnostic; they do not select a model-specific media path.

Hardware testing covers:

- E1 Zoom/E340 `main` and `sub` over TCP
- dual-lens TrackFlex `main` and `sub` on both channels over TCP and UID/LAN-P2P
- H.264, H.265, AAC, and talkback across those tests

Current Reolink specifications place [E1 Outdoor Pro](https://reolink.com/us/product/e1-outdoor-pro/), [Duo 3 PoE](https://support.reolink.com/c/duo-3-poe/), and [TrackMix PoE](https://reolink.com/us/product/reolink-trackmix-poe/) inside the same codec, profile, and talkback envelope. Other likely compatibility candidates include Duo 2 WiFi, Video Doorbell, TrackMix WiFi 6, and RLC-1212A. These are compatibility candidates, not tested support claims for this adapter.

## Configuration

```yaml
streams:
  camera_main: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main
  camera_sub: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?audio=0&backchannel=0
  camera_channel: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?channel=1
  camera_uid: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@${REOLINK_UID}/main?transport=uid
  camera_uid_routed: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@${REOLINK_UID}/main?transport=uid&local=192.0.2.10&broadcast=198.51.100.255
```

Use these URL forms:

```text
reolink://username:password@host[:port]/profile?[options]
reolink://username:password@camera-uid/profile?transport=uid[&options]
```

For LAN-P2P, put the camera UID in place of the host and add `transport=uid`. The adapter discovers that camera on the local network, then carries the Baichuan stream over reliable UDP. Escape URL-reserved characters in credentials.

Defaults are TCP port `9000`, channel `0`, the `main` profile, and enabled video, audio, and backchannel.

| Option | Values | Default | Purpose |
|---|---|---|---|
| TCP `:port` | `1`-`65535` | `9000` | Baichuan TCP port; invalid with UID transport |
| `profile` path | `main`, `sub`, `extern` | `main` | Camera stream; `extern` is a camera-dependent third profile |
| `stream` | `main`, `sub`, `extern` | `main` | Query-form alternative to the profile path; do not set both |
| `channel` | `0`-`255` | `0` | NVR or multi-lens channel |
| `transport` | `tcp`, `uid` | `tcp` | Direct TCP or local UID discovery and LAN-P2P over reliable UDP |
| `local` | IPv4 address assigned to this host | all active interfaces | Bind UID discovery and the resulting LAN-P2P socket to that address; UID only |
| `broadcast` | IPv4 address | each active interface's subnet broadcast | Override the UID discovery destination; UID only |
| `video` | `0`/`1`, `false`/`true` | enabled | Receive video from the camera |
| `audio` | `0`/`1`, `false`/`true` | enabled | Receive microphone audio from the camera |
| `backchannel` | `0`/`1` | enabled | Send microphone audio to the camera speaker |

With neither selector, UID discovery sends to each active IPv4 interface's subnet broadcast. `local` binds discovery and LAN-P2P traffic to one interface address; without `broadcast`, discovery uses that interface's subnet broadcast. `broadcast` overrides the destination. Both options are valid only with `transport=uid`. Each option may appear once; unknown options are rejected.
The profile names `mainStream`, `subStream`, `ext`, and `externStream` are also accepted for compatibility; the shorter names above are preferred.

### Audio and talkback

`audio` and `backchannel` control opposite directions. They are not redundant.

- `audio=1` receives sound from the camera microphone.
- `backchannel=1` sends sound to the camera speaker.

| `audio` | `backchannel` | Result |
|---|---|---|
| `1` | `1` | Receive camera audio and send talkback |
| `1` | `0` | Receive camera audio only |
| `0` | `1` | Send talkback only |
| `0` | `0` | No audio in either direction |

For each camera channel, choose one profile as the audio and talkback owner. We recommend the lower-resolution `sub` (fluent) profile. Set `audio=0&backchannel=0` on the other profiles. This avoids duplicate audio and makes talkback selection explicit.

Use this layout when you expose clear and fluent as separate streams:

```yaml
streams:
  camera_clear: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=0&backchannel=0
  camera_fluent: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?audio=1&backchannel=1
```

Use `camera_fluent` for clients that need talkback. Clear and fluent target the same camera speaker when they use the same `channel` value.

### Connection layout

A Reolink source uses one authenticated Baichuan connection. Its video, camera audio, and talkback share that connection. This is the recommended layout when one profile provides every required track:

```yaml
streams:
  camera: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?audio=1&backchannel=1
```

Use a second source only when you need a separate connection. The following stream gets video from `main`. It gets camera audio and talkback from a separate `sub` connection:

```yaml
streams:
  camera:
    - reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=0&backchannel=0
    - reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?video=0&audio=1&backchannel=1
```

The second source is a dedicated audio source. `video=0` prevents it from supplying video. It still receives camera audio because `audio=1`, and it supplies talkback because `backchannel=1`.

Set `audio=0` on the second source only when you need a talkback-only source. The source still opens and drains its selected preview, so use `sub` to reduce bandwidth. Either layout consumes another camera session. Use a separate connection only for firmware compatibility or diagnosis.

When several sources advertise talkback, go2rtc selects the first compatible source in listed order. Do not rely on that order. Enable `backchannel` on one source only. Separate named streams can each advertise talkback, but concurrent talk sessions on the same camera channel compete for one camera talk endpoint.

## Behavior

- One source serves all go2rtc consumers. Duplicate configured sources open separate camera sessions.
- Multiple configured sources retain go2rtc's ordered codec matching. A later H.264 source can serve a consumer that does not negotiate HEVC; `video=0` prevents a source from satisfying video.
- Connections are demand-driven. When the last consumer detaches, go2rtc stops the source and the adapter synchronously releases its preview, talk, transport, and receivers.
- The adapter does not guess a camera's session capacity. The camera remains authoritative and may reject a new source when its model-, firmware-, or client-dependent limit is reached.
- Enabled audio must start within 10 seconds. A profile reconnects after 15 seconds without a valid audio frame. It also reconnects after a 60-second window below 75% of the advertised audio clock. Set `audio=0` if audio is disabled or unwanted.
- Best-effort setup queries run concurrently per configured source and add compact model, firmware, channel, and discovery status. Failed queries do not block streaming.
- Repeated talkback negotiation, writes, and teardown are tested. Audible quality, echo, and client UI behavior remain experimental.

## Unsupported (so far)

- WAN or cloud P2P, relay, NAT traversal, and UPnP
- Battery-camera wake and cloud lifecycles
- PTZ, presets, lights, sirens, camera settings, and event APIs; existing ONVIF or camera integrations remain separate
- Automatic RTSP, RTMP, HTTP-FLV, or FFmpeg fallback

## Security and limits

- Use only on a trusted network. Baichuan does not authenticate the camera before login negotiation. It may select an unencrypted session.
- A hostile broadcast-domain peer can impersonate a camera during UID discovery and attempt offline password guessing.
- UID/LAN-P2P defaults to one IPv4 broadcast domain. Routed discovery requires directed-broadcast and return routes.
- Keep credentials in environment substitutions.
